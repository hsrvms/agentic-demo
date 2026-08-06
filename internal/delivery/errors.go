package delivery

import "errors"

var (
	ErrNoRecipients   = errors.New("no recipients specified")
	ErrEmptySubject   = errors.New("email subject must not be empty")
	ErrSMTPConfig     = errors.New("SMTP configuration is incomplete")
	ErrDeliveryFailed = errors.New("email delivery failed")
)
