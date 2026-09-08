package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/service/providerhttp"
)

type sender struct {
	phone   string
	apiKey  string
	baseURL string
	client  *http.Client
}

func New(apiKey, phone string) entity.Sender {
	return &sender{phone: phone, apiKey: apiKey, baseURL: "https://api.kavenegar.com", client: providerhttp.NewClient()}
}

func (senderValue *sender) Send(ctx context.Context, message entity.Message) (sendErr error) {
	defer func() { sendErr = providerhttp.SafeError("sms", sendErr) }()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	smsMessage, err := entity.ConcreteMessage[*entity.SMSMessage](message)
	if err != nil || smsMessage == nil || len(smsMessage.To) == 0 || smsMessage.Text == "" {
		return providerhttp.Failure("sms: invalid message")
	}
	if senderValue.apiKey == "" {
		return providerhttp.Failure("sms: API key is required")
	}
	form := url.Values{"sender": {senderValue.phone}, "receptor": {strings.Join(smsMessage.To, ",")}, "message": {smsMessage.Text + "\n"}}
	endpoint := strings.TrimRight(senderValue.baseURL, "/") + "/v1/" + url.PathEscape(senderValue.apiKey) + "/sms/send.json"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	body, err := providerhttp.Response(senderValue.client, request, "sms", http.StatusOK)
	if err != nil {
		return err
	}
	var result struct {
		Return struct {
			Status int `json:"status"`
		} `json:"return"`
		Entries []struct {
			ID     int64 `json:"messageid"`
			Status int   `json:"status"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return providerhttp.Failure("sms: invalid response")
	}
	if result.Return.Status != 200 {
		return providerhttp.Failure(fmt.Sprintf("sms: API error %d", result.Return.Status))
	}
	if len(result.Entries) != len(smsMessage.To) {
		return providerhttp.Failure("sms: incomplete acceptance response")
	}
	for _, entry := range result.Entries {
		if entry.ID <= 0 {
			return providerhttp.Failure("sms: response has no message ID")
		}
		switch entry.Status {
		case 1, 2, 4, 5, 10:
		default:
			return providerhttp.Failure(fmt.Sprintf("sms: message status %d", entry.Status))
		}
	}
	return nil
}

func (senderValue *sender) Channel() entity.Channel { return entity.ChannelSMS }
