package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
)

func TestSenderRejectsOversizedRequestBody(t *testing.T) {
	senderValue, err := New(&Config{RequestBodyMaxBytes: 3, ResponseBodyMaxBytes: -1})
	if err != nil {
		t.Fatal(err)
	}
	err = senderValue.Send(context.Background(), &entity.HTTPMessage{URL: "http://127.0.0.1", Body: "1234"})
	if !errors.Is(err, ErrBodyLimitExceeded) {
		t.Fatalf("Send() error = %v, want ErrBodyLimitExceeded", err)
	}
}

func TestSenderRejectsOversizedResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("1234"))
	}))
	defer server.Close()

	senderValue, err := newLocalSender(t, server.URL, &Config{RequestBodyMaxBytes: -1, ResponseBodyMaxBytes: 3})
	if err != nil {
		t.Fatal(err)
	}
	err = senderValue.Send(context.Background(), &entity.HTTPMessage{URL: server.URL})
	if !errors.Is(err, ErrBodyLimitExceeded) {
		t.Fatalf("Send() error = %v, want ErrBodyLimitExceeded", err)
	}
}

func TestSenderSendsHTTPMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if got := r.Header.Get("X-Test"); got != "value" {
			t.Errorf("X-Test = %q, want value", got)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type was not propagated")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	s, err := newLocalSender(t, server.URL, &Config{Timeout: time.Second, RequestBodyMaxBytes: -1, ResponseBodyMaxBytes: -1})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Send(context.Background(), &entity.HTTPMessage{
		URL:     server.URL,
		Method:  "PATCH",
		Headers: map[string]string{"X-Test": "value", "Content-Type": "application/json"},
		Body:    `{"ok":true}`,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSenderDoesNotPersistCookies(t *testing.T) {
	var seenCookie bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/set" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc", Path: "/"})
			return
		}
		cookie, err := r.Cookie("session")
		seenCookie = err == nil && cookie.Value == "abc"
	}))
	defer server.Close()

	senderValue, err := newLocalSender(t, server.URL, &Config{RequestBodyMaxBytes: -1, ResponseBodyMaxBytes: -1})
	if err != nil {
		t.Fatal(err)
	}
	s := senderValue.(*sender)
	if err := s.Send(context.Background(), &entity.HTTPMessage{URL: server.URL + "/set"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), &entity.HTTPMessage{URL: server.URL + "/check"}); err != nil {
		t.Fatal(err)
	}
	if seenCookie {
		t.Fatal("cookie leaked between submissions")
	}
}

func TestSenderReturnsNonSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusBadRequest)
	}))
	defer server.Close()

	sender, err := newLocalSender(t, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = sender.Send(context.Background(), &entity.HTTPMessage{URL: server.URL})
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("Send() error = %v, want HTTP status error", err)
	}
}

func TestSenderHonorsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender, err := newLocalSender(t, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = sender.Send(ctx, &entity.HTTPMessage{URL: server.URL})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Send() error = %v, want context.Canceled", err)
	}
}

func newLocalSender(test *testing.T, destination string, config *Config) (entity.Sender, error) {
	test.Helper()
	parsed, err := url.Parse(destination)
	if err != nil {
		test.Fatal(err)
	}
	config = config.OrDefault()
	config.AllowedDestinations = []string{parsed.Host}
	config.AllowedPrivateCIDRs = []string{"127.0.0.0/8", "::1/128"}
	return New(config)
}
