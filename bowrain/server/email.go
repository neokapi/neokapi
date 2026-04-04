// Package server — email.go
//
// EmailSenderI is kept as an alias to mailer.EmailSenderI so that test code
// and other server-package callers can reference the interface without an
// import cycle.
//
// Concrete sender implementations (SMTPSender, ResendSender) live in the
// bowrain/mailer package. initMailer wires them to Server.Mailer during
// startup.
package server

import (
	"log/slog"

	"github.com/neokapi/neokapi/bowrain/mailer"
)

// EmailSenderI is an alias for mailer.EmailSenderI so tests and server-package
// code can reference it through the server package without importing mailer
// directly.
type EmailSenderI = mailer.EmailSenderI

// initMailer builds the email sender and mailer from the server config.
// Priority: Resend API key > SMTP. If neither is configured, email features
// are disabled (Server.Mailer stays nil).
func (s *Server) initMailer(cfg Config) {
	var sender mailer.EmailSenderI

	switch {
	case cfg.ResendAPIKey != "" && cfg.SMTPFrom != "":
		sender = mailer.NewResendSender(cfg.ResendAPIKey, cfg.SMTPFrom)
		slog.Info("email: using Resend sender", "from", cfg.SMTPFrom)

	case cfg.SMTPHost != "" && cfg.SMTPFrom != "":
		smtpCfg := mailer.SMTPConfig{
			Host:     cfg.SMTPHost,
			From:     cfg.SMTPFrom,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
			UseTLS:   cfg.SMTPUseTLS,
		}
		sender = mailer.NewSMTPSender(smtpCfg)
		slog.Info("email: using SMTP sender", "host", cfg.SMTPHost, "from", cfg.SMTPFrom)

	default:
		return // email not configured
	}

	s.EmailSender = sender

	m, err := mailer.New(sender)
	if err != nil {
		slog.Warn("failed to initialize mailer (email sending disabled)", "error", err)
		return
	}
	s.Mailer = m
}
