package dto

import (
	"encoding/base64"

	"github.com/arcaptcha/kaftar/server/internal/entity"
)

type Attachment struct {
	Name        string `json:"name" binding:"required"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Data        string `json:"data" binding:"required"`
}

func AttachmentToEntity(atts ...Attachment) ([]entity.Attachment, error) {
	attachments := make([]entity.Attachment, 0, len(atts))

	for _, att := range atts {
		if att.Name == "" || att.Data == "" {
			continue
		}

		// Decode base64 data
		data, err := base64.StdEncoding.DecodeString(att.Data)
		if err != nil {
			return nil, err
		}

		contentType := att.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		attachments = append(attachments, entity.Attachment{
			Name:        att.Name,
			ContentType: contentType,
			Size:        att.Size,
			Data:        data,
		})
	}
	return attachments, nil
}
