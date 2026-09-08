package email

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/smtp"
	"strconv"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/service/providerhttp"
	"gopkg.in/gomail.v2"
)

type smtpDialer struct{ config Config }

func (dialer *smtpDialer) DialAndSend(ctx context.Context, messages ...*gomail.Message) (sendErr error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	defer func() {
		if sendErr != nil && ctx.Err() != nil {
			sendErr = ctx.Err()
		} else if deadline, exists := ctx.Deadline(); sendErr != nil && exists && !time.Now().Before(deadline) {
			sendErr = context.DeadlineExceeded
		}
	}()
	address := net.JoinHostPort(dialer.config.Host, strconv.Itoa(int(dialer.config.Port)))
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer connection.Close()
	rawConnection := connection
	stop := context.AfterFunc(ctx, func() { _ = rawConnection.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := connection.SetDeadline(deadline); err != nil {
		return err
	}
	tlsConfig := &tls.Config{ServerName: dialer.config.Host, MinVersion: tls.VersionTLS12}
	if dialer.config.Port == 465 {
		tlsConnection := tls.Client(connection, tlsConfig)
		if err := tlsConnection.HandshakeContext(ctx); err != nil {
			return err
		}
		connection = tlsConnection
	}
	client, err := smtp.NewClient(connection, dialer.config.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if dialer.config.Port != 465 {
		if supportsTLS, _ := client.Extension("STARTTLS"); supportsTLS {
			if err := client.StartTLS(tlsConfig); err != nil {
				return err
			}
		} else if dialer.config.Username != "" {
			return providerhttp.Failure("email: TLS is required for authentication")
		}
	}
	if dialer.config.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", dialer.config.Username, dialer.config.Password, dialer.config.Host)); err != nil {
			return err
		}
	}
	err = gomail.Send(gomail.SendFunc(func(from string, recipients []string, message io.WriterTo) error {
		if err := client.Mail(from); err != nil {
			return err
		}
		for _, recipient := range recipients {
			if err := client.Rcpt(recipient); err != nil {
				return err
			}
		}
		writer, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := message.WriteTo(writer); err != nil {
			return err
		}
		return writer.Close()
	}), messages...)
	if err != nil {
		return err
	}
	_ = client.Quit()
	return nil
}
