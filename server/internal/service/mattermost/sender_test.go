package mattermost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arcaptcha/kaftar/server/internal/entity"
)

func TestSenderPostsTextMessage(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v4/posts" {
			test.Fatalf("path = %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer token" {
			test.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		var payload postRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			test.Fatal(err)
		}
		if payload.ChannelID != "channel" || payload.Message != "title\nbody" {
			test.Fatalf("payload = %+v", payload)
		}
		response.WriteHeader(http.StatusCreated)
		_, _ = response.Write([]byte(`{"id":"post-id"}`))
	}))
	defer server.Close()

	senderValue := &sender{baseURL: server.URL, pat: "token", client: server.Client()}
	err := senderValue.Send(context.Background(), &entity.MattermostMessage{
		ChannelID: "channel",
		Title:     "title",
		Text:      "body",
	})
	if err != nil {
		test.Fatal(err)
	}
}

func TestSenderUploadsFirstAttachment(test *testing.T) {
	var uploaded bool
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v4/files":
			if err := request.ParseMultipartForm(1024); err != nil {
				test.Fatal(err)
			}
			if request.FormValue("channel_id") != "channel" {
				test.Fatalf("channel_id = %q", request.FormValue("channel_id"))
			}
			uploaded = true
			response.WriteHeader(http.StatusCreated)
			_, _ = response.Write([]byte(`{"file_infos":[{"id":"file-id"}]}`))
		case "/api/v4/posts":
			if !uploaded {
				test.Fatal("post happened before upload")
			}
			var payload postRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				test.Fatal(err)
			}
			if len(payload.FileIDs) != 1 || payload.FileIDs[0] != "file-id" {
				test.Fatalf("file IDs = %v", payload.FileIDs)
			}
			response.WriteHeader(http.StatusCreated)
			_, _ = response.Write([]byte(`{"id":"post-id"}`))
		default:
			test.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	senderValue := &sender{baseURL: server.URL, pat: "token", client: server.Client()}
	err := senderValue.Send(context.Background(), &entity.MattermostMessage{
		ChannelID: "channel",
		Title:     "report",
		Attachments: []entity.Attachment{
			{Name: "report.txt", Data: []byte("content")},
		},
	})
	if err != nil {
		test.Fatal(err)
	}
}
