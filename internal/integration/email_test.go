package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Flawed-Four/muni-backend/internal/domain"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/email"
)

var mailpitClient = &http.Client{Timeout: 5 * time.Second}

type mailpitAddress struct {
	Name    string
	Address string
}

type mailpitMessage struct {
	ID      string
	From    mailpitAddress
	To      []mailpitAddress
	Bcc     []mailpitAddress
	Subject string
	Text    string // only set by the single-message endpoint
	HTML    string // only set by the single-message endpoint
}

// resetMail empties the Mailpit inbox so every test starts from a clean state.
func resetMail(t *testing.T) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, mailpitURL+"/api/v1/messages", nil)
	if err != nil {
		t.Fatalf("build delete request: %v", err)
	}
	resp, err := mailpitClient.Do(req)
	if err != nil {
		t.Fatalf("clear mailpit: %v (is it running? start it with `make test-db-up`)", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear mailpit: status %d", resp.StatusCode)
	}
}

func mailpitGet(t *testing.T, path string, out any) {
	t.Helper()
	resp, err := mailpitClient.Get(mailpitURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func inbox(t *testing.T) []mailpitMessage {
	t.Helper()
	var list struct{ Messages []mailpitMessage }
	mailpitGet(t, "/api/v1/messages", &list)
	return list.Messages
}

func newSender(t *testing.T) *email.SMTPSender {
	t.Helper()
	sender, err := email.NewSMTPSender(testCfg)
	if err != nil {
		t.Fatalf("NewSMTPSender: %v", err)
	}
	return sender
}

func TestSMTPSender_DeliversTextAndHTML(t *testing.T) {
	resetMail(t)

	const subject = "Verifikasi email Anda — Muni ✓"
	err := newSender(t).Send(context.Background(), domain.Email{
		To:      "user@example.com",
		Subject: subject,
		Text:    "Klik tautan berikut.",
		HTML:    "<p>Klik <b>tautan</b> berikut.</p>",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	messages := inbox(t)
	if len(messages) != 1 {
		t.Fatalf("inbox has %d messages, want 1", len(messages))
	}

	var got mailpitMessage
	mailpitGet(t, "/api/v1/message/"+messages[0].ID, &got)

	if got.From.Address != "noreply@muni.test" || got.From.Name != "Muni Test" {
		t.Fatalf("From = %+v", got.From)
	}
	if len(got.To) != 1 || got.To[0].Address != "user@example.com" {
		t.Fatalf("To = %+v", got.To)
	}
	if got.Subject != subject {
		t.Fatalf("Subject = %q, want %q", got.Subject, subject)
	}
	if strings.TrimSpace(got.Text) != "Klik tautan berikut." {
		t.Fatalf("Text = %q", got.Text)
	}
	if strings.TrimSpace(got.HTML) != "<p>Klik <b>tautan</b> berikut.</p>" {
		t.Fatalf("HTML = %q", got.HTML)
	}
}

func TestSMTPSender_TextOnly(t *testing.T) {
	resetMail(t)

	err := newSender(t).Send(context.Background(), domain.Email{
		To: "user@example.com", Subject: "Halo", Text: "Isi pesan",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	messages := inbox(t)
	if len(messages) != 1 {
		t.Fatalf("inbox has %d messages, want 1", len(messages))
	}

	var got mailpitMessage
	mailpitGet(t, "/api/v1/message/"+messages[0].ID, &got)
	if strings.TrimSpace(got.Text) != "Isi pesan" || got.HTML != "" {
		t.Fatalf("Text = %q, HTML = %q", got.Text, got.HTML)
	}
}

func TestSMTPSender_RejectsInvalidEmail(t *testing.T) {
	valid := domain.Email{To: "user@example.com", Subject: "Halo", Text: "Isi pesan"}

	tests := []struct {
		name   string
		mutate func(*domain.Email)
	}{
		{"empty recipient", func(e *domain.Email) { e.To = "" }},
		{"malformed recipient", func(e *domain.Email) { e.To = "not-an-address" }},
		{"several recipients", func(e *domain.Email) { e.To = "a@example.com, b@example.com" }},
		{"header injection in recipient", func(e *domain.Email) { e.To = "a@example.com\r\nBcc: evil@example.com" }},
		{"header injection in subject", func(e *domain.Email) { e.Subject = "Halo\r\nBcc: evil@example.com" }},
		{"line feed in subject", func(e *domain.Email) { e.Subject = "Halo\nBcc: evil@example.com" }},
		{"empty text body", func(e *domain.Email) { e.Text = "" }},
	}

	sender := newSender(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetMail(t)

			msg := valid
			tt.mutate(&msg)
			if err := sender.Send(context.Background(), msg); err == nil {
				t.Fatal("Send succeeded, want an error")
			}
			if n := len(inbox(t)); n != 0 {
				t.Fatalf("inbox has %d messages, want 0", n)
			}
		})
	}
}

func TestSMTPSender_CanceledContext(t *testing.T) {
	resetMail(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := newSender(t).Send(ctx, domain.Email{To: "user@example.com", Subject: "Halo", Text: "Isi pesan"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want context.Canceled", err)
	}
	if n := len(inbox(t)); n != 0 {
		t.Fatalf("inbox has %d messages, want 0", n)
	}
}

// Mailpit offers no STARTTLS here, so a sender that demands it must refuse to
// fall back to plaintext.
func TestSMTPSender_STARTTLSDoesNotFallBackToPlaintext(t *testing.T) {
	resetMail(t)

	cfg := *testCfg
	cfg.SMTP.TLSMode = "starttls"
	sender, err := email.NewSMTPSender(&cfg)
	if err != nil {
		t.Fatalf("NewSMTPSender: %v", err)
	}

	err = sender.Send(context.Background(), domain.Email{To: "user@example.com", Subject: "Halo", Text: "Isi pesan"})
	if err == nil {
		t.Fatal("Send succeeded over plaintext, want an error")
	}
	if n := len(inbox(t)); n != 0 {
		t.Fatalf("inbox has %d messages, want 0", n)
	}
}

func TestSMTPSender_SharedBetweenGoroutines(t *testing.T) {
	resetMail(t)

	const count = 10
	sender := newSender(t)
	errs := make([]error, count)

	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			errs[i] = sender.Send(context.Background(), domain.Email{
				To:      fmt.Sprintf("user%d@example.com", i),
				Subject: fmt.Sprintf("Pesan %d", i),
				Text:    "Isi pesan",
			})
		})
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		t.Fatalf("Send: %v", err)
	}

	subjects := make(map[string]bool)
	for _, m := range inbox(t) {
		subjects[m.Subject] = true
	}
	if len(subjects) != count {
		t.Fatalf("got %d distinct messages, want %d", len(subjects), count)
	}
}

func TestNewSMTPSender_InvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		tlsMode string
		from    string
	}{
		{"empty from address", "none", ""},
		{"malformed from address", "none", "not-an-address"},
		{"unknown tls mode", "ssl", "noreply@muni.test"},
		{"empty tls mode", "", "noreply@muni.test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := *testCfg
			cfg.SMTP.TLSMode = tt.tlsMode
			cfg.SMTP.FromAddress = tt.from

			if _, err := email.NewSMTPSender(&cfg); err == nil {
				t.Fatal("NewSMTPSender succeeded, want an error")
			}
		})
	}
}
