package email

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"

	"go.uber.org/zap"
	"gopkg.in/gomail.v2"

	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/service/providerhttp"
)

type Config struct {
	Host        string
	Port        uint16
	Username    string
	Password    string
	FromAddress string
}

type messageDialer interface {
	DialAndSend(context.Context, ...*gomail.Message) error
}

type sender struct {
	dialer      messageDialer
	fromAddress string
}

func New(config Config, _ *zap.Logger) (entity.Sender, error) {
	if config.Host == "" {
		return nil, errors.New("host is required")
	}
	if config.Port == 0 {
		return nil, errors.New("port is required")
	}
	if config.Username == "" {
		return nil, errors.New("username is required")
	}
	if config.Password == "" {
		return nil, errors.New("password is required")
	}
	if config.FromAddress != "" {
		if _, err := mail.ParseAddress(config.FromAddress); err != nil {
			return nil, fmt.Errorf("invalid from address: %w", err)
		}
	}

	fromAddress := config.FromAddress
	if fromAddress == "" {
		fromAddress = config.Username
	}

	return &sender{
		dialer:      &smtpDialer{config: config},
		fromAddress: fromAddress,
	}, nil
}

func (senderValue *sender) Send(ctx context.Context, message entity.Message) (sendErr error) {
	defer func() { sendErr = providerhttp.SafeError("email", sendErr) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	emailMessage, err := senderValue.typeCheck(message)
	if err != nil {
		return err
	}
	if err := emailMessage.Validate(); err != nil {
		return err
	}

	mailMessage := gomail.NewMessage()
	mailMessage.SetHeader("From", senderValue.fromAddress)
	mailMessage.SetHeader("To", emailMessage.To...)
	mailMessage.SetHeader("Subject", emailMessage.Subject)
	if len(emailMessage.CC) > 0 {
		mailMessage.SetHeader("Cc", emailMessage.CC...)
	}
	if len(emailMessage.BCC) > 0 {
		mailMessage.SetHeader("Bcc", emailMessage.BCC...)
	}

	if emailMessage.HTMLBody != "" {
		mailMessage.SetBody("text/html", emailMessage.HTMLBody)
	} else {
		mailMessage.SetBody("text/plain", emailMessage.TextBody)
	}

	for attachmentIndex := range emailMessage.Attachments {
		attachment := emailMessage.Attachments[attachmentIndex]
		if len(attachment.Data) == 0 {
			continue
		}
		settings := []gomail.FileSetting{
			gomail.SetCopyFunc(func(writer io.Writer) error {
				_, err := writer.Write(attachment.Data)
				return err
			}),
		}
		if attachment.ContentType != "" {
			settings = append(settings, gomail.SetHeader(map[string][]string{
				"Content-Type": {attachment.ContentType},
			}))
		}
		mailMessage.Attach(attachment.Name, settings...)
	}

	if err := senderValue.dialer.DialAndSend(ctx, mailMessage); err != nil {
		return err
	}
	return nil
}

func (senderValue *sender) Channel() entity.Channel {
	return entity.ChannelEmail
}

func (senderValue *sender) typeCheck(message entity.Message) (*entity.EmailMessage, error) {
	emailMessage, err := entity.ConcreteMessage[*entity.EmailMessage](message)
	if err != nil {
		return nil, err
	}
	if emailMessage == nil {
		return nil, errors.New("nil message")
	}
	return emailMessage, nil
}
