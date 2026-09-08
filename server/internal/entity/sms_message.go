package entity

import (
	"errors"
	"slices"
)

type SMSMessage struct {
	To   []string `json:"to"`
	Text string   `json:"text"`
}

func (m *SMSMessage) Channel() Channel {
	return ChannelSMS
}

func (m *SMSMessage) Validate() error {
	m.To = slices.DeleteFunc(m.To, func(s string) bool { return s == "" })
	m.To = slices.Compact(m.To)
	if len(m.To) == 0 {
		return errors.New("sms: missing recipient")
	}
	if m.Text == "" {
		return errors.New("sms: missing text body")
	}
	return nil
}
