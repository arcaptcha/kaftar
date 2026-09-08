package dto

import (
	"github.com/arcaptcha/kaftar/server/internal/entity"
)

type MattermostMessage struct {
	ChannelID   string       `json:"channel_id" binding:"required"`
	Text        string       `json:"text"`
	Title       string       `json:"title,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// ToEntity converts the DTO to an entity, decoding base64 attachment data.
func (m *MattermostMessage) ToEntity() (*entity.MattermostMessage, error) {
	attachments, err := AttachmentToEntity(m.Attachments...)
	if err != nil {
		return nil, err
	}
	return &entity.MattermostMessage{
		ChannelID:   m.ChannelID,
		Text:        m.Text,
		Title:       m.Title,
		Attachments: attachments,
	}, nil
}

type SendMattermostResult struct {
	ID string `json:"id"`
}

type StatusMattermostRequest struct {
	ID string `json:"id" query:"id" param:"id"`
}
