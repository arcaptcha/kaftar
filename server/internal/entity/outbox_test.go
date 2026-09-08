package entity

import (
	"encoding/json"
	"log"
	"testing"
)

func TestOutbox(t *testing.T) {

	smsMessage := &SMSMessage{
		To:   []string{"100"},
		Text: "test message",
	}
	payload, err := json.Marshal(smsMessage)
	if err != nil {
		t.Fatalf("failed to marshal SMS message: %v", err)
	}

	outbox := &Outbox{
		Channel:    ChannelSMS,
		Payload:    payload,
		State:      OutboxStatePending,
		MaxRetries: 10,
	}

	outboxBytes, err := json.Marshal(outbox)
	if err != nil {
		t.Fatalf("failed to marshal outbox: %v", err)
	}

	log.Printf("outbox: %s", outboxBytes)
}
