package delivery

import (
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

// smtpService implements DeliveryService using net/smtp.
type smtpService struct {
	config SMTPConfig
	logger *slog.Logger
}

// NewSMTPService creates a DeliveryService that sends emails via SMTP.
func NewSMTPService(cfg SMTPConfig, logger *slog.Logger) DeliveryService {
	return &smtpService{config: cfg, logger: logger}
}

func (s *smtpService) Deliver(_ context.Context, params DeliverParams) error {
	if len(params.To) == 0 {
		return ErrNoRecipients
	}
	if strings.TrimSpace(params.Subject) == "" {
		return ErrEmptySubject
	}
	if s.config.Host == "" {
		return ErrSMTPConfig
	}

	msg := buildMessage(s.config.From, params.To, params.Subject, params.Body)

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	var auth smtp.Auth
	if s.config.Username != "" {
		auth = smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
	}

	if err := smtp.SendMail(addr, auth, s.config.From, params.To, msg); err != nil {
		s.logger.Error("SMTP send failed",
			"to", params.To,
			"subject", params.Subject,
			"error", err,
		)
		return fmt.Errorf("%w: %v", ErrDeliveryFailed, err)
	}

	s.logger.Info("email delivered",
		"to", params.To,
		"subject", params.Subject,
	)
	return nil
}

// buildMessage constructs an RFC 2822 email message.
func buildMessage(from string, to []string, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// LogService is a DeliveryService that logs emails instead of sending them.
// Useful for development and testing when no SMTP server is available.
type LogService struct {
	Logger *slog.Logger
}

// NewLogService creates a DeliveryService that logs emails to stdout.
func NewLogService(logger *slog.Logger) DeliveryService {
	return &LogService{Logger: logger}
}

func (s *LogService) Deliver(_ context.Context, params DeliverParams) error {
	if len(params.To) == 0 {
		return ErrNoRecipients
	}
	s.Logger.Info("delivery (log mode): would send email",
		"to", params.To,
		"subject", params.Subject,
		"body_len", len(params.Body),
	)
	return nil
}
