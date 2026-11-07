package domain

import "context"

// Email is a message for a single recipient. Each recipient gets their own
// message so addresses are never exposed to one another.
type Email struct {
	To      string
	Subject string
	Text    string // required, the plain-text body
	HTML    string // optional, sent as an alternative to Text
}

// EmailSender delivers transactional email. Send blocks until the mail server
// has accepted the message.
type EmailSender interface {
	Send(ctx context.Context, email Email) error
}
