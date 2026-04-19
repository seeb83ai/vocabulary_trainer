package email

import (
	"fmt"
	"net/smtp"
	"os"
	"strings"
)

// Sender sends emails via SMTP. A nil Sender is a no-op (dev mode).
type Sender struct {
	host     string
	port     string
	user     string
	password string
	from     string
}

// NewSenderFromEnv reads SMTP_HOST, SMTP_PORT (default 587), SMTP_USER,
// SMTP_PASS, and SMTP_FROM from the environment. Returns nil if SMTP_HOST
// is not set — callers treat nil as "email disabled".
func NewSenderFromEnv() *Sender {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		return nil
	}
	port := os.Getenv("SMTP_PORT")
	if port == "" {
		port = "587"
	}
	from := os.Getenv("SMTP_FROM")
	if from == "" {
		from = os.Getenv("SMTP_USER")
	}
	return &Sender{
		host:     host,
		port:     port,
		user:     os.Getenv("SMTP_USER"),
		password: os.Getenv("SMTP_PASS"),
		from:     from,
	}
}

// Send delivers a plain-text + HTML email. bodyHTML may be empty to send
// text-only. Uses SMTP AUTH LOGIN with STARTTLS when credentials are set.
func (s *Sender) Send(to, subject, bodyText, bodyHTML string) error {
	addr := s.host + ":" + s.port

	var auth smtp.Auth
	if s.user != "" && s.password != "" {
		auth = smtp.PlainAuth("", s.user, s.password, s.host)
	}

	var msg strings.Builder
	boundary := "boundary42vocabtrainer"
	msg.WriteString("From: " + s.from + "\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: " + subject + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")

	if bodyHTML != "" {
		msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		msg.WriteString(bodyText + "\r\n\r\n")
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
		msg.WriteString(bodyHTML + "\r\n\r\n")
		msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		msg.WriteString(bodyText + "\r\n")
	}

	return smtp.SendMail(addr, auth, s.from, []string{to}, []byte(msg.String()))
}
