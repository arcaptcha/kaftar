package bale

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/service/providerhttp"
)

const baleTextLimit = 4000

type Config struct {
	BotToken string
}

type sender struct {
	botToken string
	baseURL  string
	client   *http.Client
}

func New(config Config) (entity.Sender, error) {
	if strings.TrimPrefix(config.BotToken, "bot") == "" || strings.ContainsAny(config.BotToken, "/?# \t\r\n") {
		return nil, errors.New("bot token can not be empty")
	}
	return &sender{
		botToken: config.BotToken,
		baseURL:  "https://tapi.bale.ai",
		client:   providerhttp.NewClient(),
	}, nil
}

func (senderValue *sender) Send(ctx context.Context, message entity.Message) (sendErr error) {
	defer func() { sendErr = providerhttp.SafeError("bale", sendErr) }()
	baleMessage, err := entity.ConcreteMessage[*entity.BaleMessage](message)
	if err != nil {
		return err
	}
	if baleMessage == nil {
		return errors.New("nil message")
	}
	if baleMessage.ChatID == "" {
		return errors.New("chat_id can not be empty")
	}
	if len(baleMessage.Attachments) > 1 {
		return providerhttp.Failure("bale: at most one attachment is supported")
	}
	if len(baleMessage.Attachments) == 1 && len(baleMessage.Attachments[0].Data) == 0 {
		return providerhttp.Failure("bale: attachment is empty")
	}

	text := baleMessage.Text
	if text != "" {
		text += "\n"
	}
	if len(baleMessage.Attachments) == 0 && len(text) <= baleTextLimit {
		return senderValue.sendMessage(ctx, baleMessage.ChatID, text)
	}

	filename := "message.txt"
	data := []byte(text)
	caption := ""
	if len(baleMessage.Attachments) > 0 {
		attachment := baleMessage.Attachments[0]
		filename = attachment.Name
		data = attachment.Data
		caption = baleMessage.Text
	}
	return senderValue.sendDocument(ctx, baleMessage.ChatID, caption, filename, data)
}

func (senderValue *sender) Channel() entity.Channel {
	return entity.ChannelBale
}

func (senderValue *sender) sendMessage(ctx context.Context, chatID, text string) error {
	body, err := json.Marshal(map[string]string{"chat_id": chatID, "text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, senderValue.endpoint("sendMessage"), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return senderValue.do(req)
}

func (senderValue *sender) sendDocument(ctx context.Context, chatID, caption, filename string, data []byte) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("chat_id", chatID); err != nil {
		return err
	}
	if err := writer.WriteField("caption", caption); err != nil {
		return err
	}
	part, err := writer.CreateFormFile("document", filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, senderValue.endpoint("sendDocument"), &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return senderValue.do(req)
}

func (senderValue *sender) endpoint(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", strings.TrimRight(senderValue.baseURL, "/"), url.PathEscape(strings.TrimPrefix(senderValue.botToken, "bot")), method)
}

func (senderValue *sender) do(req *http.Request) error {
	body, err := providerhttp.Response(senderValue.client, req, "bale", 0)
	if err != nil {
		return err
	}
	var result struct {
		OK        bool            `json:"ok"`
		Result    json.RawMessage `json:"result"`
		ErrorCode int             `json:"error_code"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return providerhttp.Failure("bale: invalid response")
	}
	if !result.OK {
		return providerhttp.Failure(fmt.Sprintf("bale: API error %d", result.ErrorCode))
	}
	var message struct {
		ID int64 `json:"message_id"`
	}
	if err := json.Unmarshal(result.Result, &message); err != nil || message.ID == 0 {
		return providerhttp.Failure("bale: response has no message ID")
	}
	return nil
}
