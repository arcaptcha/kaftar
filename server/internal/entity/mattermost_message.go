package entity

import "errors"

type MattermostMessage struct {
	ChannelID   string       `json:"channel_id"`
	Text        string       `json:"text"`
	Title       string       `json:"title,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

func (m *MattermostMessage) Channel() Channel {
	return ChannelMattermost
}

func (m *MattermostMessage) Validate() error {
	if m.Text == "" && m.Title == "" {
		return errors.New("mattermost: missing text or title")
	}
	if m.ChannelID == "" {
		return errors.New("mattermost: missing channel ID")
	}
	return nil
}
