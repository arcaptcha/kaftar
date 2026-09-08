package dto

import "github.com/arcaptcha/kaftar/server/internal/entity"

type HTTPMessage struct {
	URL     string            `json:"url" binding:"required"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

func (m *HTTPMessage) ToEntity() *entity.HTTPMessage {
	return &entity.HTTPMessage{
		URL:     m.URL,
		Method:  m.Method,
		Headers: m.Headers,
		Body:    m.Body,
	}
}

type SendHTTPResult struct {
	ID string `json:"id"`
}

type StatusHTTPRequest struct {
	ID string `json:"id" query:"id" param:"id"`
}
