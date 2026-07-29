package delivery

import "context"

// DeliverParams holds the inputs for sending an email.
type DeliverParams struct {
	To      []string
	Subject string
	Body    string // plain text or HTML
}

// DeliveryService sends emails. The queue DeliveryHandler uses this
// to deliver reports to recipients.
type DeliveryService interface {
	Deliver(ctx context.Context, params DeliverParams) error
}

// SMTPConfig holds SMTP server configuration.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}
