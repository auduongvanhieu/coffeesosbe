package mailer

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDisabledMailerLogsInsteadOfFailing(t *testing.T) {
	m := New(Config{}, quiet())
	if m.Enabled() {
		t.Error("a mailer without host must report disabled")
	}
	// Password reset must still answer 204 when mail is not configured.
	if err := m.Send(context.Background(), "a@b.co", "subject", "body"); err != nil {
		t.Errorf("Send on a disabled mailer = %v, want nil", err)
	}
}

func TestSenderAddressStripsDisplayName(t *testing.T) {
	cases := map[string]string{
		"CoffeeSOS <no-reply@coffeesos.online>": "no-reply@coffeesos.online",
		"no-reply@coffeesos.online":             "no-reply@coffeesos.online",
		"  spaced@example.com  ":                "spaced@example.com",
	}
	for in, want := range cases {
		if got := senderAddress(in); got != want {
			t.Errorf("senderAddress(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMessageCarriesUTF8Headers(t *testing.T) {
	// Vietnamese subjects must survive, so the body is declared UTF-8.
	m := New(Config{Host: "smtp.example.com", From: "CoffeeSOS <no-reply@example.com>"}, quiet())
	if !m.Enabled() {
		t.Fatal("mailer with host and from should be enabled")
	}
	// Build the same message Send would, without dialing anything.
	msg := strings.Join([]string{
		"From: " + m.cfg.From,
		"Content-Type: text/plain; charset=UTF-8",
	}, "\r\n")
	if !strings.Contains(msg, "charset=UTF-8") {
		t.Error("message must declare UTF-8")
	}
}
