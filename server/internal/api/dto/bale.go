package dto

import "github.com/arcaptcha/kaftar/server/internal/entity"

type BaleMessage struct {
	ChatID      string       `json:"chat_id" binding:"required"`
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// ToEntity converts the DTO to an entity, decoding base64 attachment data
func (m *BaleMessage) ToEntity() (*entity.BaleMessage, error) {
	attachments, err := AttachmentToEntity(m.Attachments...)
	if err != nil {
		return nil, err
	}

	return &entity.BaleMessage{
		ChatID:      m.ChatID,
		Text:        m.Text,
		Attachments: attachments,
	}, nil
}

type SendBaleResult struct {
	ID string `json:"id"`
}

type StatusBaleRequest struct {
	ID string `json:"id" query:"id" param:"id"`
}
