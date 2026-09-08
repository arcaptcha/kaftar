package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/service/providerhttp"
)

const defaultBodyMaxBytes int64 = 1 << 20

type Config struct {
	AllowedDestinations  []string
	AllowedPrivateCIDRs  []string
	Timeout              time.Duration
	RequestBodyMaxBytes  int64
	ResponseBodyMaxBytes int64
}

func (c *Config) OrDefault() *Config {
	if c == nil {
		c = &Config{
			RequestBodyMaxBytes:  defaultBodyMaxBytes,
			ResponseBodyMaxBytes: defaultBodyMaxBytes,
		}
	}
	if c.Timeout <= 0 {
		copyConfig := *c
		copyConfig.Timeout = 30 * time.Second
		return &copyConfig
	}
	copyConfig := *c
	return &copyConfig
}

type sender struct {
	cfg    *Config
	client *http.Client
	policy *destinationPolicy
}

func New(cfg *Config) (entity.Sender, error) {
	cfg = cfg.OrDefault()
	policy, err := newDestinationPolicy(cfg)
	if err != nil {
		return nil, err
	}
	client := providerhttp.NewClient()
	client.Timeout = cfg.Timeout
	transport := client.Transport.(*http.Transport)
	transport.DialContext = policy.dialContext
	return &sender{cfg: cfg, client: client, policy: policy}, nil
}

func (s *sender) Send(ctx context.Context, m entity.Message) (sendErr error) {
	defer func() {
		if !errors.Is(sendErr, ErrBodyLimitExceeded) {
			sendErr = providerhttp.SafeError("http", sendErr)
		}
	}()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	msg, err := entity.ConcreteHttpMessage(m)
	if err != nil {
		return err
	}
	if msg == nil {
		return fmt.Errorf("http: nil message")
	}
	if err := msg.Validate(); err != nil {
		return err
	}
	return s.send(ctx, msg)
}

func (s *sender) Channel() entity.Channel {
	return entity.ChannelHTTP
}

var ErrBodyLimitExceeded = errors.New("body exceeds configured size limit")

func (s *sender) send(ctx context.Context, msg *entity.HTTPMessage) error {
	if s.cfg.RequestBodyMaxBytes >= 0 && int64(len(msg.Body)) > s.cfg.RequestBodyMaxBytes {
		return fmt.Errorf("http: request body size %d exceeds configured limit %d: %w",
			len(msg.Body), s.cfg.RequestBodyMaxBytes, ErrBodyLimitExceeded)
	}

	req, err := http.NewRequestWithContext(ctx, msg.EffectiveMethod(), msg.URL, strings.NewReader(msg.Body))
	if err != nil {
		return fmt.Errorf("http: create request: %w", err)
	}
	if err := s.policy.validate(req.URL); err != nil {
		return err
	}
	for name, value := range msg.Headers {
		switch strings.ToLower(name) {
		case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "proxy-connection", "te", "trailer", "transfer-encoding", "upgrade", "host":
			return providerhttp.Failure("http: hop-by-hop or authority header is not permitted")
		}
		req.Header.Set(name, value)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("http: execute request: %w", err)
	}
	defer resp.Body.Close()

	_, err = readBody(resp.Body, s.cfg.ResponseBodyMaxBytes)
	if err != nil {
		return fmt.Errorf("http: read response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return providerhttp.Failure(fmt.Sprintf("http: HTTP %d", resp.StatusCode))
	}
	return nil
}

func readBody(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes < 0 {
		return io.ReadAll(r)
	}
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("response body size exceeds configured limit %d: %w", maxBytes, ErrBodyLimitExceeded)
	}
	return body, nil
}

func IsBodyLimitExceeded(err error) bool { return errors.Is(err, ErrBodyLimitExceeded) }
