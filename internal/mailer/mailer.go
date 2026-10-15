// Package mailer sends the few transactional emails CoffeeSOS needs (today:
// password resets). When SMTP is not configured it logs the message instead,
// so a dev machine or a fresh server still works end to end.
package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type Config struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string // "CoffeeSOS <no-reply@coffeesos.online>"
}

type Mailer struct {
	cfg Config
	log *slog.Logger
}

func New(cfg Config, log *slog.Logger) *Mailer {
	return &Mailer{cfg: cfg, log: log}
}

// Enabled reports whether real mail can be sent.
func (m *Mailer) Enabled() bool {
	return m.cfg.Host != "" && m.cfg.From != ""
}

// Send delivers a plain-text email. A failure is returned to the caller, which
// decides whether it matters; password reset answers the same either way so an
// outage never reveals which addresses exist.
func (m *Mailer) Send(ctx context.Context, to, subject, body string) error {
	if !m.Enabled() {
		m.log.Warn("mailer not configured; email not sent",
			"to", to, "subject", subject, "body", body)
		return nil
	}
	port := m.cfg.Port
	if port == "" {
		port = "587"
	}
	addr := net.JoinHostPort(m.cfg.Host, port)

	msg := strings.Join([]string{
		"From: " + m.cfg.From,
		"To: " + to,
		"Subject: " + subject,
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	}
	if err := smtp.SendMail(addr, auth, senderAddress(m.cfg.From), []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	return nil
}

// senderAddress pulls the bare address out of "Name <a@b.c>".
func senderAddress(from string) string {
	if i := strings.LastIndex(from, "<"); i >= 0 {
		if j := strings.LastIndex(from, ">"); j > i {
			return from[i+1 : j]
		}
	}
	return strings.TrimSpace(from)
}
