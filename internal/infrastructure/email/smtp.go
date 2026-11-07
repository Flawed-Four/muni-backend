// Package email delivers transactional email over SMTP.
package email

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wneessen/go-mail"

	"github.com/Flawed-Four/muni-backend/internal/config"
	"github.com/Flawed-Four/muni-backend/internal/domain"
)

// Values accepted by SMTP_TLS_MODE.
const (
	tlsModeSTARTTLS = "starttls" // upgrade the connection, fail if the server cannot (port 587)
	tlsModeImplicit = "tls"      // TLS from the first byte (port 465)
	tlsModeNone     = "none"     // plaintext, for a local mail catcher only
)

// SMTPSender delivers email through one SMTP relay. Every Send opens its own
// connection, so a single sender can be shared between goroutines.
type SMTPSender struct {
	client      *mail.Client
	fromName    string
	fromAddress string
}

// NewSMTPSender validates the configuration without contacting the server.
func NewSMTPSender(cfg *config.Config) (*SMTPSender, error) {
	c := cfg.SMTP

	if err := mail.NewMsg().FromFormat(c.FromName, c.FromAddress); err != nil {
		return nil, fmt.Errorf("invalid SMTP_FROM_ADDRESS %q: %w", c.FromAddress, err)
	}

	opts := []mail.Option{mail.WithPort(c.Port), mail.WithTimeout(c.Timeout)}
	switch c.TLSMode {
	case tlsModeSTARTTLS:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	case tlsModeImplicit:
		opts = append(opts, mail.WithSSL())
	case tlsModeNone:
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	default:
		return nil, fmt.Errorf("invalid SMTP_TLS_MODE %q: want %s, %s or %s",
			c.TLSMode, tlsModeSTARTTLS, tlsModeImplicit, tlsModeNone)
	}
	if c.Username != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
			mail.WithUsername(c.Username),
			mail.WithPassword(c.Password),
		)
	}

	client, err := mail.NewClient(c.Host, opts...)
	if err != nil {
		return nil, fmt.Errorf("create smtp client: %w", err)
	}
	return &SMTPSender{client: client, fromName: c.FromName, fromAddress: c.FromAddress}, nil
}

// Send delivers e, returning once the server has accepted it.
func (s *SMTPSender) Send(ctx context.Context, e domain.Email) error {
	if e.Text == "" {
		return errors.New("send email: text body is required")
	}
	// go-mail encodes line breaks, so this is not what stops header injection.
	// A multi-line subject is always a caller bug, so fail instead of sending it.
	if strings.ContainsAny(e.Subject, "\r\n") {
		return errors.New("send email: subject must not contain line breaks")
	}

	msg := mail.NewMsg()
	if err := msg.FromFormat(s.fromName, s.fromAddress); err != nil {
		return fmt.Errorf("send email: set sender: %w", err)
	}
	if err := msg.To(e.To); err != nil {
		return fmt.Errorf("send email: set recipient: %w", err)
	}
	msg.Subject(e.Subject)
	msg.SetBodyString(mail.TypeTextPlain, e.Text)
	if e.HTML != "" {
		msg.AddAlternativeString(mail.TypeTextHTML, e.HTML)
	}

	if err := s.client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}
