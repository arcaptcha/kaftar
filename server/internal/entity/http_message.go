package entity

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

// HTTPMessage describes an HTTP request to be delivered.
// Body is sent as-is; callers that need a structured payload should JSON-encode it first.
type HTTPMessage struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

var _ Message = &HTTPMessage{}

func (m *HTTPMessage) Channel() Channel {
	return ChannelHTTP
}

// EffectiveMethod returns the configured method or POST when it is omitted.
func (m *HTTPMessage) EffectiveMethod() string {
	if m == nil || strings.TrimSpace(m.Method) == "" {
		return http.MethodPost
	}
	return strings.ToUpper(strings.TrimSpace(m.Method))
}

func (m *HTTPMessage) Validate() error {
	if m == nil {
		return errors.New("http: message is nil")
	}

	u, err := url.Parse(strings.TrimSpace(m.URL))
	if err != nil {
		return fmt.Errorf("http: invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("http: URL must use http or https")
	}
	if u.Host == "" {
		return errors.New("http: URL must include a host")
	}

	if _, err := http.NewRequest(m.EffectiveMethod(), u.String(), nil); err != nil {
		return fmt.Errorf("http: invalid request: %w", err)
	}

	for name, value := range m.Headers {
		if !validHeaderName(name) {
			return fmt.Errorf("http: invalid header name %q", name)
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("http: invalid value for header %q: contains invalid chars %q", name, []rune("\r\n"))
		}
	}

	return nil
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		default:
			return false
		}
	}
	return true
}

func ConcreteHttpMessage(msg Message) (*HTTPMessage, error) {
	return ConcreteMessage[*HTTPMessage](msg)
}
