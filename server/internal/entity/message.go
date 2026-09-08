package entity

import (
	"encoding/json"
	"fmt"
)

type Message interface {
	Channel() Channel
	Validate() error
}

func ConcreteMessage[M any](m Message) (M, error) {
	if msg, ok := m.(M); ok {
		return msg, nil
	}
	var zero M
	return zero, fmt.Errorf("type assertion failed: need %T, got %T", zero, m)
}

func UnmarshalMessage(channel Channel, data []byte) (Message, error) {
	switch channel {
	case ChannelSMS:
		var msg SMSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return &msg, nil
	case ChannelEmail:
		var msg EmailMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return &msg, nil
	case ChannelMattermost:
		var msg MattermostMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return &msg, nil
	case ChannelBale:
		var msg BaleMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return &msg, nil
	case ChannelHTTP:
		var msg HTTPMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return &msg, nil
	default:
		return nil, fmt.Errorf("invalid or unimplemented channel (%v)", channel)
	}
}
