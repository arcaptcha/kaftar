package entity

import (
	"encoding/json"
	"log"
	"testing"
)

func TestSMSMessage(t *testing.T) {

	smsMessage := &SMSMessage{
		To:   []string{"100"},
		Text: "test message",
	}

	payload, err := json.Marshal(smsMessage)
	if err != nil {
		t.Fatalf("failed to marshal SMS message: %v", err)
	}

	log.Printf("payload: %s", string(payload))
}
