package mattermost

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arcaptcha/kaftar/server/internal/entity"
)

func TestMattermostRejectsFailedPosts(test *testing.T) {
	for _, status := range []int{200, 201, 401, 500} {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			response.WriteHeader(status)
			_, _ = response.Write([]byte(`{"error":"synthetic-secret"}`))
		}))
		sender := &sender{baseURL: server.URL, pat: "synthetic-token", client: server.Client()}
		err := sender.Send(context.Background(), &entity.MattermostMessage{ChannelID: "chat", Text: "hello"})
		server.Close()
		if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
			test.Fatalf("status %d: %v", status, err)
		}
	}
}

func TestMattermostBoundariesAndAttachmentPreservation(test *testing.T) {
	for _, text := range []string{strings.Repeat("x", 499), strings.Repeat("x", 500), strings.Repeat("x", 501), strings.Repeat("é", 251)} {
		uploads := 0
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			response.WriteHeader(http.StatusCreated)
			if request.URL.Path == "/api/v4/files" {
				uploads++
				file, _, err := request.FormFile("files")
				if err != nil {
					test.Error(err)
					return
				}
				defer file.Close()
				body, err := io.ReadAll(file)
				if err != nil || string(body) != text {
					test.Error("attachment content changed")
				}
				_, _ = response.Write([]byte(`{"file_infos":[{"id":"file"}]}`))
			} else {
				_, _ = response.Write([]byte(`{"id":"post"}`))
			}
		}))
		sender := &sender{baseURL: server.URL, pat: "synthetic-token", client: server.Client()}
		if err := sender.Send(context.Background(), &entity.MattermostMessage{ChannelID: "chat", Text: text}); err != nil {
			test.Error(err)
		}
		server.Close()
		if (uploads == 1) != (len(text) > mattermostTextLimit) {
			test.Error("incorrect long-text boundary")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	sender := &sender{baseURL: server.URL, pat: "synthetic-token", client: server.Client()}
	message := &entity.MattermostMessage{ChannelID: "chat", Title: "title", Attachments: []entity.Attachment{{Name: "file", Data: []byte("content")}}}
	if err := sender.Send(context.Background(), message); err == nil {
		test.Fatal("failed upload accepted")
	}
	if string(message.Attachments[0].Data) != "content" {
		test.Fatal("failure mutated attachment")
	}
	message.Attachments = append(message.Attachments, message.Attachments[0])
	if err := sender.Send(context.Background(), message); err == nil {
		test.Fatal("multiple attachments accepted")
	}
}
