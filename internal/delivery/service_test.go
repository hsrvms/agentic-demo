package delivery

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
)

func TestLogService_Deliver(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	svc := NewLogService(logger)

	err := svc.Deliver(context.Background(), DeliverParams{
		To:      []string{"user@example.com"},
		Subject: "Daily Report — Jul 29, 2025",
		Body:    "# Report\n\nRevenue is up.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLogService_NoRecipients(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	svc := NewLogService(logger)

	err := svc.Deliver(context.Background(), DeliverParams{
		To:      nil,
		Subject: "test",
		Body:    "test",
	})
	if !errors.Is(err, ErrNoRecipients) {
		t.Errorf("error = %v, want %v", err, ErrNoRecipients)
	}
}

func TestSMTPService_NoRecipients(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	svc := NewSMTPService(SMTPConfig{Host: "smtp.example.com", Port: 587}, logger)

	err := svc.Deliver(context.Background(), DeliverParams{
		To:      nil,
		Subject: "test",
		Body:    "test",
	})
	if !errors.Is(err, ErrNoRecipients) {
		t.Errorf("error = %v, want %v", err, ErrNoRecipients)
	}
}

func TestSMTPService_EmptySubject(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	svc := NewSMTPService(SMTPConfig{Host: "smtp.example.com", Port: 587}, logger)

	err := svc.Deliver(context.Background(), DeliverParams{
		To:      []string{"user@example.com"},
		Subject: "",
		Body:    "test",
	})
	if !errors.Is(err, ErrEmptySubject) {
		t.Errorf("error = %v, want %v", err, ErrEmptySubject)
	}
}

func TestSMTPService_MissingHost(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	svc := NewSMTPService(SMTPConfig{}, logger)

	err := svc.Deliver(context.Background(), DeliverParams{
		To:      []string{"user@example.com"},
		Subject: "test",
		Body:    "test",
	})
	if !errors.Is(err, ErrSMTPConfig) {
		t.Errorf("error = %v, want %v", err, ErrSMTPConfig)
	}
}

func TestBuildMessage(t *testing.T) {
	msg := buildMessage(
		"noreply@platform.com",
		[]string{"user@example.com"},
		"Daily Report",
		"Hello world",
	)
	s := string(msg)

	if !contains(s, "From: noreply@platform.com") {
		t.Error("missing From header")
	}
	if !contains(s, "To: user@example.com") {
		t.Error("missing To header")
	}
	if !contains(s, "Subject: Daily Report") {
		t.Error("missing Subject header")
	}
	if !contains(s, "Content-Type: text/plain") {
		t.Error("missing Content-Type header")
	}
	if !contains(s, "Hello world") {
		t.Error("missing body")
	}
}

func TestBuildMessage_MultipleRecipients(t *testing.T) {
	msg := buildMessage(
		"noreply@platform.com",
		[]string{"a@example.com", "b@example.com"},
		"Test",
		"body",
	)
	s := string(msg)

	if !contains(s, "To: a@example.com, b@example.com") {
		t.Error("missing combined To header")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
