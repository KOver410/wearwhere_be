package mailer

import "context"

// Mailer abstracts away the transport so we can swap SMTP / SendGrid / etc.
type Mailer interface {
	Send(ctx context.Context, to, subject, htmlBody string) error
}
