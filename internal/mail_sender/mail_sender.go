package mail_sender

import (
	"fmt"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
)

type MailKind string

type MailSenderConfig struct {
	Host     string
	Email    string
	Password string
	MailsDir string
}

type MailSender struct {
	From              string
	Host              string
	Auth              smtp.Auth
	ResetPasswordMail MailKind
}

func New(cfg *MailSenderConfig) *MailSender {
	auth := smtp.PlainAuth("", cfg.Email, cfg.Password, cfg.Host)

	return &MailSender{
		Host:              cfg.Host,
		Auth:              auth,
		From:              cfg.Email,
		ResetPasswordMail: MailKind(filepath.Join(cfg.MailsDir, "reset-password.html")),
	}
}

func (m *MailSender) SendMail(subject string, to []string, kind MailKind, vars map[string]string) error {
	bytesContent, err := os.ReadFile(string(kind))
	if err != nil {
		return err
	}
	content := fmt.Sprintf("Subject: %s\r\n", subject) +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n\r\n" +
		string(bytesContent)

	for k, v := range vars {
		content = strings.ReplaceAll(content, fmt.Sprintf("{{%s}}", k), v)
	}
	err = smtp.SendMail(m.Host+":587", m.Auth, m.From, to, []byte(content))
	if err != nil {
		return err
	}
	return nil
}
