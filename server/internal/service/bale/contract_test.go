package bale

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arcaptcha/kaftar/server/internal/entity"
)

func TestBaleResponseFailures(test *testing.T) {
	for _, body := range []string{`{"ok":false,"error_code":400,"description":"synthetic-secret"}`, `{"ok":true}`, `{"ok":true,"result":null}`, `{`, `null`} {
		test.Run(body, func(test *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) { _, _ = response.Write([]byte(body)) }))
			defer server.Close()
			sender := &sender{botToken: "test-token", baseURL: server.URL, client: server.Client()}
			err := sender.Send(context.Background(), &entity.BaleMessage{ChatID: "chat", Text: "hello"})
			if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
				test.Fatalf("unsafe result: %v", err)
			}
		})
	}
}

type canceledTransport struct{}

func (canceledTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, context.Canceled
}

func TestBaleHTTPRejectionAndCredentialSafety(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusBadRequest)
		_, _ = response.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()
	sender := &sender{botToken: "synthetic-secret", baseURL: server.URL, client: server.Client()}
	message := &entity.BaleMessage{ChatID: "chat", Text: "hello"}
	if err := sender.Send(context.Background(), message); err == nil {
		test.Fatal("HTTP 400 counted as success")
	}
	sender.client = &http.Client{Transport: canceledTransport{}}
	err := sender.Send(context.Background(), message)
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), sender.botToken) {
		test.Fatalf("unsafe cancellation result = %v", err)
	}
}

func TestBaleTextBoundariesAndAttachmentSafety(test *testing.T) {
	for _, text := range []string{strings.Repeat("x", 3998), strings.Repeat("x", 3999), strings.Repeat("x", 4000), strings.Repeat("é", 2000)} {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			want := "/bottest-token/sendMessage"
			if len(text)+1 > baleTextLimit {
				want = "/bottest-token/sendDocument"
				file, _, err := request.FormFile("document")
				if err != nil {
					test.Error(err)
					return
				}
				defer file.Close()
				body, err := io.ReadAll(file)
				if err != nil || string(body) != text+"\n" {
					test.Error("document text changed")
				}
			}
			if request.URL.Path != want {
				test.Errorf("path = %s", request.URL.Path)
			}
			_, _ = response.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
		}))
		sender := &sender{botToken: "bottest-token", baseURL: server.URL, client: server.Client()}
		if err := sender.Send(context.Background(), &entity.BaleMessage{ChatID: "chat", Text: text}); err != nil {
			test.Error(err)
		}
		server.Close()
	}
	sender := &sender{botToken: "test-token"}
	if err := sender.Send(context.Background(), &entity.BaleMessage{ChatID: "chat", Attachments: []entity.Attachment{{Name: "one", Data: []byte("one")}, {Name: "two", Data: []byte("two")}}}); err == nil {
		test.Fatal("multiple attachments silently accepted")
	}
}
