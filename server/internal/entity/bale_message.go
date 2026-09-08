package entity

var _ Message = &BaleMessage{}

type BaleMessage struct {
	ChatID      string       `json:"chat_id"`
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

func (m *BaleMessage) Channel() Channel {
	return ChannelBale
}

func (m *BaleMessage) Validate() error {
	return nil
}
