package dto

import (
	"github.com/arcaptcha/kaftar/server/internal/entity"
)

type EmailMessage struct {
	To          []string     `json:"to" binding:"required"`
	CC          []string     `json:"cc,omitempty"`
	BCC         []string     `json:"bcc,omitempty"`
	Subject     string       `json:"subject" binding:"required"`
	HTMLBody    string       `json:"html_body,omitempty"`
	TextBody    string       `json:"text_body,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

func (m *EmailMessage) ToEntity() (*entity.EmailMessage, error) {
	attachments, err := AttachmentToEntity(m.Attachments...)
	if err != nil {
		return nil, err
	}

	return &entity.EmailMessage{
		To:          m.To,
		CC:          m.CC,
		BCC:         m.BCC,
		Subject:     m.Subject,
		HTMLBody:    m.HTMLBody,
		TextBody:    m.TextBody,
		Attachments: attachments,
	}, nil
}

type SendEmailResult struct {
	ID string `json:"id"`
}

type StatusEmailRequest struct {
	ID string `json:"id" query:"id" param:"id"`
}
