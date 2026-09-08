package providerhttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const ResponseLimit int64 = 1 << 20

type Failure string

func (failure Failure) Error() string { return string(failure) }

func SafeError(provider string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s: %w", provider, context.Canceled)
	}
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
		return fmt.Errorf("%s: %w", provider, context.DeadlineExceeded)
	}
	var safe Failure
	if errors.As(err, &safe) {
		return safe
	}
	return Failure(provider + ": request failed")
}

func NewClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Timeout: 30 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func Response(client *http.Client, request *http.Request, provider string, expectedStatus int) ([]byte, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, SafeError(provider, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 || (expectedStatus != 0 && response.StatusCode != expectedStatus) {
		return nil, Failure(fmt.Sprintf("%s: HTTP %d", provider, response.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, ResponseLimit+1))
	if err != nil {
		return nil, SafeError(provider, err)
	}
	if int64(len(body)) > ResponseLimit {
		return nil, Failure(provider + ": response too large")
	}
	return body, nil
}
