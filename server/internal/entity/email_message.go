package entity

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

type EmailMessage struct {
	To          []string     `json:"to"`
	CC          []string     `json:"cc,omitempty"`
	BCC         []string     `json:"bcc,omitempty"`
	Subject     string       `json:"subject"`
	HTMLBody    string       `json:"html_body,omitempty"`
	TextBody    string       `json:"text_body,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

func (m *EmailMessage) Channel() Channel {
	return ChannelEmail
}

func (m *EmailMessage) Validate() error {
	if len(m.To) == 0 {
		return errors.New("email: missing recipient")
	}
	if m.Subject == "" {
		return errors.New("email: missing subject")
	}
	return nil
}

// Attachment represents a file attachment with metadata and data
type Attachment struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Data        []byte `json:"data,omitempty"` // Base64-encoded in JSON, raw bytes in memory
}

// MarshalJSON implements custom JSON marshaling to base64-encode attachment data
func (a Attachment) MarshalJSON() ([]byte, error) {
	type Alias Attachment
	return json.Marshal(&struct {
		Data string `json:"data,omitempty"`
		*Alias
	}{
		Data:  base64.StdEncoding.EncodeToString(a.Data),
		Alias: (*Alias)(&a),
	})
}

// UnmarshalJSON implements custom JSON unmarshaling to decode base64 attachment data
func (a *Attachment) UnmarshalJSON(data []byte) error {
	type Alias Attachment
	aux := &struct {
		Data string `json:"data,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(a),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Decode base64 data if present
	if aux.Data != "" {
		decoded, err := base64.StdEncoding.DecodeString(aux.Data)
		if err != nil {
			return err
		}
		a.Data = decoded
	}

	return nil
}
