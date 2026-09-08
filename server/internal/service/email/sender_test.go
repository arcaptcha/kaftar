package email

import (
	"context"
	"errors"
	"testing"

	"gopkg.in/gomail.v2"

	"github.com/arcaptcha/kaftar/server/internal/entity"
)

type fakeDialer struct {
	message *gomail.Message
	err     error
}

func (dialerValue *fakeDialer) DialAndSend(_ context.Context, messages ...*gomail.Message) error {
	if len(messages) > 0 {
		dialerValue.message = messages[0]
	}
	return dialerValue.err
}

func TestSenderBuildsEmailHeaders(test *testing.T) {
	dialer := &fakeDialer{}
	senderValue := &sender{dialer: dialer, fromAddress: "from@example.com"}

	err := senderValue.Send(context.Background(), &entity.EmailMessage{
		To:       []string{"to@example.com"},
		CC:       []string{"cc@example.com"},
		BCC:      []string{"bcc@example.com"},
		Subject:  "subject",
		TextBody: "body",
	})
	if err != nil {
		test.Fatal(err)
	}
	if got := dialer.message.GetHeader("From"); len(got) != 1 || got[0] != "from@example.com" {
		test.Fatalf("From = %v", got)
	}
	if got := dialer.message.GetHeader("Cc"); len(got) != 1 || got[0] != "cc@example.com" {
		test.Fatalf("Cc = %v", got)
	}
	if got := dialer.message.GetHeader("Bcc"); len(got) != 1 || got[0] != "bcc@example.com" {
		test.Fatalf("Bcc = %v", got)
	}
}

func TestSenderReturnsDialError(test *testing.T) {
	want := errors.New("dial failed")
	senderValue := &sender{dialer: &fakeDialer{err: want}, fromAddress: "from@example.com"}
	err := senderValue.Send(context.Background(), &entity.EmailMessage{
		To:      []string{"to@example.com"},
		Subject: "subject",
	})
	if err == nil || err.Error() != "email: request failed" {
		test.Fatalf("Send() error = %v, want %v", err, want)
	}
}
