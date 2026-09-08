package email

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/entity"
)

type smtpCapture struct {
	recipients []string
	body       []byte
	err        error
}

func TestSMTPEnvelopeAndMIME(test *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		test.Fatal(err)
	}
	defer listener.Close()
	result := make(chan smtpCapture, 1)
	go func() {
		capture := smtpCapture{}
		defer func() { result <- capture }()
		connection, err := listener.Accept()
		if err != nil {
			capture.err = err
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
		reader := textproto.NewReader(bufio.NewReader(connection))
		_, _ = io.WriteString(connection, "220 localhost SMTP\r\n")
		for {
			line, err := reader.ReadLine()
			if err != nil {
				capture.err = err
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				_, _ = io.WriteString(connection, "250-localhost\r\n250 HELP\r\n")
			case strings.HasPrefix(line, "MAIL FROM:"):
				_, _ = io.WriteString(connection, "250 ok\r\n")
			case strings.HasPrefix(line, "RCPT TO:"):
				capture.recipients = append(capture.recipients, strings.TrimPrefix(line, "RCPT TO:"))
				_, _ = io.WriteString(connection, "250 ok\r\n")
			case line == "DATA":
				_, _ = io.WriteString(connection, "354 data\r\n")
				capture.body, capture.err = reader.ReadDotBytes()
				if capture.err != nil {
					return
				}
				_, _ = io.WriteString(connection, "250 accepted\r\n")
			case line == "QUIT":
				_, _ = io.WriteString(connection, "221 bye\r\n")
				return
			default:
				capture.err = errors.New("unexpected SMTP command")
				return
			}
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	sender := &sender{dialer: &smtpDialer{config: Config{Host: host, Port: uint16(portNumber)}}, fromAddress: "from@example.com"}
	message := &entity.EmailMessage{To: []string{"to@example.com"}, CC: []string{"cc@example.com"}, BCC: []string{"bcc@example.com"}, Subject: "subject", TextBody: "plain", HTMLBody: "<b>html</b>", Attachments: []entity.Attachment{{Name: "report.txt", ContentType: "text/plain", Data: []byte("attachment bytes")}}}
	if err := sender.Send(context.Background(), message); err != nil {
		test.Fatal(err)
	}
	capture := <-result
	if capture.err != nil {
		test.Fatal(capture.err)
	}
	if strings.Join(capture.recipients, ",") != "<to@example.com>,<cc@example.com>,<bcc@example.com>" {
		test.Fatalf("recipients = %v", capture.recipients)
	}
	parsed, err := mail.ReadMessage(strings.NewReader(string(capture.body)))
	if err != nil {
		test.Fatal(err)
	}
	if parsed.Header.Get("Bcc") != "" {
		test.Fatal("BCC leaked into transmitted headers")
	}
	mediaType, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		test.Fatal("expected multipart email")
	}
	reader := multipart.NewReader(parsed.Body, params["boundary"])
	first, err := reader.NextPart()
	if err != nil {
		test.Fatal(err)
	}
	body, err := io.ReadAll(first)
	if err != nil || !strings.Contains(first.Header.Get("Content-Type"), "text/html") || string(body) != "<b>html</b>" {
		test.Fatalf("HTML part = %s", body)
	}
	attachment, err := reader.NextPart()
	if err != nil {
		test.Fatal(err)
	}
	var decoded io.Reader = attachment
	if attachment.Header.Get("Content-Transfer-Encoding") == "base64" {
		decoded = base64.NewDecoder(base64.StdEncoding, attachment)
	} else if attachment.Header.Get("Content-Transfer-Encoding") == "quoted-printable" {
		decoded = quotedprintable.NewReader(attachment)
	}
	body, err = io.ReadAll(decoded)
	if err != nil || attachment.FileName() != "report.txt" || string(body) != "attachment bytes" {
		test.Fatal("attachment MIME changed")
	}
	if string(message.Attachments[0].Data) != "attachment bytes" {
		test.Fatal("caller attachment mutated")
	}
}

func TestSMTPCancellationClosesStalledConnection(test *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		test.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{})
	closed := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			closed <- err
			return
		}
		defer connection.Close()
		close(accepted)
		_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, err = connection.Read(make([]byte, 1))
		closed <- err
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	sender := &sender{dialer: &smtpDialer{config: Config{Host: host, Port: uint16(portNumber)}}, fromAddress: "from@example.com"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		finished <- sender.Send(ctx, &entity.EmailMessage{To: []string{"to@example.com"}, Subject: "subject"})
	}()
	select {
	case <-accepted:
	case <-time.After(3 * time.Second):
		test.Fatal("SMTP connection not established")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			test.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		test.Fatal("SMTP send ignored cancellation")
	}
	if err := <-closed; !errors.Is(err, io.EOF) {
		test.Fatalf("connection not closed on cancellation: %v", err)
	}
}
