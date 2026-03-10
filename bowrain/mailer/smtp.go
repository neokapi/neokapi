package mailer

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	mail "github.com/wneessen/go-mail"
)

// SMTPConfig holds the configuration for the SMTP sender.
type SMTPConfig struct {
	// Host is the SMTP server hostname (without port).
	Host string

	// Port is the SMTP server port (25, 465, 587, 1025 …).
	// Defaults to 587 when zero.
	Port int

	// From is the sender email address (e.g. "noreply@bowrain.cloud").
	From string

	// Username and Password are used for SMTP authentication.
	// Leave both empty for unauthenticated relay (e.g. local Mailpit).
	Username string
	Password string

	// UseTLS enables implicit TLS (SMTPS, typically port 465).
	// When false, STARTTLS is attempted if available (TLSOpportunistic).
	// For completely plaintext connections (local dev) set both UseTLS=false
	// and no Username/Password — the sender will use NoTLS automatically.
	UseTLS bool
}

// SMTPSender implements EmailSenderI using go-mail with full SMTP auth and
// TLS support. It replaces the stdlib net/smtp sender which had no auth.
type SMTPSender struct {
	cfg SMTPConfig
}

// NewSMTPSender creates an SMTPSender with the given configuration.
// It accepts the legacy "host:port" string for backward compatibility.
func NewSMTPSender(cfg SMTPConfig) *SMTPSender {
	// Support legacy "host:port" strings in the Host field.
	if strings.Contains(cfg.Host, ":") {
		host, portStr, err := splitHostPort(cfg.Host)
		if err == nil {
			cfg.Host = host
			if cfg.Port == 0 {
				cfg.Port, _ = strconv.Atoi(portStr)
			}
		}
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	return &SMTPSender{cfg: cfg}
}

// Send sends an HTML email via SMTP.
func (s *SMTPSender) Send(_ context.Context, to, subject, htmlBody string) error {
	m := mail.NewMsg()
	if err := m.From(s.cfg.From); err != nil {
		return fmt.Errorf("set From: %w", err)
	}
	if err := m.To(to); err != nil {
		return fmt.Errorf("set To: %w", err)
	}
	m.Subject(subject)
	m.SetBodyString(mail.TypeTextHTML, htmlBody)

	clientOpts := s.buildClientOptions()
	c, err := mail.NewClient(s.cfg.Host, clientOpts...)
	if err != nil {
		return fmt.Errorf("create SMTP client for %s:%d: %w", s.cfg.Host, s.cfg.Port, err)
	}

	if err := c.DialAndSend(m); err != nil {
		return fmt.Errorf("send email to %s via %s:%d: %w", to, s.cfg.Host, s.cfg.Port, err)
	}
	return nil
}

func (s *SMTPSender) buildClientOptions() []mail.Option {
	opts := []mail.Option{
		mail.WithPort(s.cfg.Port),
	}

	switch {
	case s.cfg.UseTLS:
		// Implicit TLS — wrap the connection from the start (port 465).
		opts = append(opts, mail.WithSSL())
		if s.cfg.Username != "" {
			opts = append(opts,
				mail.WithSMTPAuth(mail.SMTPAuthPlain),
				mail.WithUsername(s.cfg.Username),
				mail.WithPassword(s.cfg.Password),
			)
		}
	case s.cfg.Username != "":
		// STARTTLS with authentication (port 587).
		opts = append(opts,
			mail.WithTLSPolicy(mail.TLSOpportunistic),
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(s.cfg.Username),
			mail.WithPassword(s.cfg.Password),
		)
	default:
		// Unauthenticated relay (local dev, e.g. Mailpit on port 1025).
		opts = append(opts,
			mail.WithTLSPolicy(mail.NoTLS),
			mail.WithSMTPAuth(mail.SMTPAuthNoAuth),
		)
	}
	return opts
}

// splitHostPort is a thin wrapper around net.SplitHostPort-like behaviour
// that doesn't require importing net.
func splitHostPort(hostport string) (host, port string, err error) {
	idx := strings.LastIndex(hostport, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("missing port in %q", hostport)
	}
	return hostport[:idx], hostport[idx+1:], nil
}
