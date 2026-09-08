package mattermost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/service/providerhttp"
)

const mattermostTextLimit = 500

type Config struct {
	MattermostURL string
	PAT           string
}

type sender struct {
	baseURL string
	pat     string
	client  *http.Client
}

type postRequest struct {
	ChannelID string   `json:"channel_id"`
	Message   string   `json:"message"`
	FileIDs   []string `json:"file_ids,omitempty"`
}

func New(config Config) (entity.Sender, error) {
	if config.MattermostURL == "" {
		return nil, errors.New("mattermost URL is required")
	}
	if config.PAT == "" {
		return nil, errors.New("mattermost PAT (Personal Access Token) is required")
	}
	parsed, err := url.Parse(config.MattermostURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, errors.New("invalid Mattermost URL")
	}
	return &sender{
		baseURL: strings.TrimRight(config.MattermostURL, "/"),
		pat:     config.PAT,
		client:  providerhttp.NewClient(),
	}, nil
}

func (senderValue *sender) Send(ctx context.Context, message entity.Message) (sendErr error) {
	defer func() { sendErr = providerhttp.SafeError("mattermost", sendErr) }()
	mattermostMessage, err := senderValue.typeCheck(message)
	if err != nil {
		return err
	}
	if err := mattermostMessage.Validate(); err != nil {
		return err
	}
	if len(mattermostMessage.Attachments) > 1 {
		return providerhttp.Failure("mattermost: at most one attachment is supported")
	}

	text := mattermostMessage.Text
	if mattermostMessage.Title != "" && mattermostMessage.Text != "" {
		text = mattermostMessage.Title + "\n" + mattermostMessage.Text
	} else if mattermostMessage.Title != "" {
		text = mattermostMessage.Title
	}

	if len(mattermostMessage.Attachments) == 0 && len(text) <= mattermostTextLimit {
		return senderValue.post(ctx, postRequest{ChannelID: mattermostMessage.ChannelID, Message: text})
	}

	filename := "message.txt"
	data := []byte(text)
	if len(mattermostMessage.Attachments) > 0 {
		for _, candidate := range mattermostMessage.Attachments {
			if len(candidate.Data) == 0 {
				return errors.New("attachment must have data")
			}
		}
		attachment := mattermostMessage.Attachments[0]
		filename = attachment.Name
		data = attachment.Data
	}

	fileID, err := senderValue.upload(ctx, mattermostMessage.ChannelID, filename, data)
	if err != nil {
		return err
	}
	return senderValue.post(ctx, postRequest{
		ChannelID: mattermostMessage.ChannelID,
		Message:   mattermostMessage.Title,
		FileIDs:   []string{fileID},
	})
}

func (senderValue *sender) Channel() entity.Channel {
	return entity.ChannelMattermost
}

func (senderValue *sender) typeCheck(message entity.Message) (*entity.MattermostMessage, error) {
	mattermostMessage, err := entity.ConcreteMessage[*entity.MattermostMessage](message)
	if err != nil {
		return nil, err
	}
	if mattermostMessage == nil {
		return nil, errors.New("nil message")
	}
	return mattermostMessage, nil
}

func (senderValue *sender) post(ctx context.Context, payload postRequest) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, senderValue.baseURL+"/api/v4/posts", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+senderValue.pat)
	req.Header.Set("Content-Type", "application/json")

	responseBody, err := providerhttp.Response(senderValue.client, req, "mattermost post", http.StatusCreated)
	if err != nil {
		return err
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil || result.ID == "" {
		return providerhttp.Failure("mattermost: invalid post response")
	}
	return nil
}

func (senderValue *sender) upload(ctx context.Context, channelID, filename string, data []byte) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("channel_id", channelID); err != nil {
		return "", err
	}
	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, senderValue.baseURL+"/api/v4/files", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+senderValue.pat)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	responseBody, err := providerhttp.Response(senderValue.client, req, "mattermost upload", http.StatusCreated)
	if err != nil {
		return "", err
	}

	var result struct {
		FileInfos []struct {
			ID string `json:"id"`
		} `json:"file_infos"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return "", err
	}
	if len(result.FileInfos) == 0 || result.FileInfos[0].ID == "" {
		return "", errors.New("mattermost file upload returned no file ID")
	}
	return result.FileInfos[0].ID, nil
}
