package sms

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
)

func TestSenderUsesConfiguredPhoneAndRecipients(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			test.Error(err)
		}
		if request.URL.Path != "/v1/test-key/sms/send.json" || request.Form.Get("sender") != "sender" || request.Form.Get("receptor") != "100,200" || request.Form.Get("message") != "hello\n" {
			test.Error("incorrect SMS request")
		}
		_, _ = response.Write([]byte(`{"return":{"status":200},"entries":[{"messageid":1,"status":1},{"messageid":2,"status":1}]}`))
	}))
	defer server.Close()
	senderValue := &sender{phone: "sender", apiKey: "test-key", baseURL: server.URL, client: server.Client()}
	if err := senderValue.Send(context.Background(), &entity.SMSMessage{To: []string{"100", "200"}, Text: "hello"}); err != nil {
		test.Fatal(err)
	}
}

func TestSenderRejectsMissingRecipients(test *testing.T) {
	if err := New("synthetic-key", "sender").Send(context.Background(), &entity.SMSMessage{Text: "hello"}); err == nil {
		test.Fatal("expected missing recipient error")
	}
}

func TestSenderRejectsFailureResponses(test *testing.T) {
	for _, body := range []string{`{`, `null`, `{"return":{"status":401}}`, `{"return":{"status":200},"entries":[]}`, `{"return":{"status":200},"entries":[{"messageid":1,"status":6}]}`} {
		test.Run(body, func(test *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) { _, _ = response.Write([]byte(body)) }))
			defer server.Close()
			senderValue := &sender{apiKey: "synthetic-key", baseURL: server.URL, client: server.Client()}
			if err := senderValue.Send(context.Background(), &entity.SMSMessage{To: []string{"100"}, Text: "hello"}); err == nil {
				test.Fatal("failed response counted as success")
			}
		})
	}
}

func TestSMSInFlightCancellationAndCredentialSafety(test *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		close(entered)
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	senderValue := &sender{apiKey: "synthetic-secret", baseURL: server.URL, client: server.Client()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- senderValue.Send(ctx, &entity.SMSMessage{To: []string{"100"}, Text: "hello"}) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		test.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), senderValue.apiKey) {
			test.Fatalf("unsafe cancellation result = %v", err)
		}
	case <-time.After(time.Second):
		test.Fatal("SMS request ignored cancellation")
	}
}
