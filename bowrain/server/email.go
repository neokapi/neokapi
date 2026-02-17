package server

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// EmailSenderI sends email messages.
type EmailSenderI interface {
	Send(ctx context.Context, to, subject, htmlBody string) error
}

// SMTPSender implements EmailSenderI using net/smtp.
type SMTPSender struct {
	Host string // host:port
	From string // sender email address
}

// Send sends an HTML email via SMTP.
func (s *SMTPSender) Send(_ context.Context, to, subject, htmlBody string) error {
	host, _, err := net.SplitHostPort(s.Host)
	if err != nil {
		return fmt.Errorf("invalid SMTP host %q: %w", s.Host, err)
	}

	msg := strings.Join([]string{
		"From: " + s.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		htmlBody,
	}, "\r\n")

	// For local dev (e.g., Mailpit), no auth is needed.
	if err := smtp.SendMail(s.Host, nil, s.From, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("send email to %s via %s: %w", to, host, err)
	}
	return nil
}
