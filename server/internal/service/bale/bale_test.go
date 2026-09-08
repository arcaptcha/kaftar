package bale

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arcaptcha/kaftar/server/internal/entity"
)

func TestSenderPostsTextMessage(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/bottest-token/sendMessage" {
			test.Fatalf("path = %s", request.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			test.Fatal(err)
		}
		if payload["chat_id"] != "chat" || payload["text"] != "hello\n" {
			test.Fatalf("payload = %v", payload)
		}
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()

	senderValue := &sender{botToken: "test-token", baseURL: server.URL, client: server.Client()}
	if err := senderValue.Send(context.Background(), &entity.BaleMessage{ChatID: "chat", Text: "hello"}); err != nil {
		test.Fatal(err)
	}
}

func TestSenderPostsFirstAttachment(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/bottest-token/sendDocument" {
			test.Fatalf("path = %s", request.URL.Path)
		}
		if err := request.ParseMultipartForm(1024); err != nil {
			test.Fatal(err)
		}
		if request.FormValue("chat_id") != "chat" || request.FormValue("caption") != "caption" {
			test.Fatalf("form = %v", request.MultipartForm.Value)
		}
		file, header, err := request.FormFile("document")
		if err != nil {
			test.Fatal(err)
		}
		defer file.Close()
		if header.Filename != "report.txt" {
			test.Fatalf("filename = %q", header.Filename)
		}
		content, err := io.ReadAll(file)
		if err != nil || string(content) != "content" {
			test.Fatal("attachment content changed")
		}
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()

	senderValue := &sender{botToken: "test-token", baseURL: server.URL, client: server.Client()}
	err := senderValue.Send(context.Background(), &entity.BaleMessage{
		ChatID: "chat",
		Text:   "caption",
		Attachments: []entity.Attachment{
			{Name: "report.txt", Data: []byte("content")},
		},
	})
	if err != nil {
		test.Fatal(err)
	}
}

func TestNewRejectsEmptyToken(test *testing.T) {
	if _, err := New(Config{}); err == nil {
		test.Fatal("expected empty token error")
	}
}
