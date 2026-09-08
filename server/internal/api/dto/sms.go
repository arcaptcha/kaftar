package dto

import "github.com/arcaptcha/kaftar/server/internal/entity"

type SMSMessage struct {
	To   []string `json:"to"`
	Text string   `json:"text"`
}

func (m *SMSMessage) ToEntity() *entity.SMSMessage {
	return &entity.SMSMessage{To: m.To, Text: m.Text}
}

type SendSMSResult struct {
	ID string `json:"id"`
}

type StatusSMSRequest struct {
	ID string `json:"id" query:"id" param:"id"`
}
