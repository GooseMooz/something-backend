package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/goosemooz/something-backend/config"
)

type SMTPMailer struct {
	host     string
	port     int
	username string
	password string
	from     string
}

func NewSMTPMailer(cfg *config.Config) (*SMTPMailer, error) {
	if cfg.SMTPHost == "" || cfg.SMTPUsername == "" || cfg.SMTPPassword == "" || cfg.SMTPFrom == "" {
		return nil, fmt.Errorf("smtp mailer is not fully configured")
	}
	if cfg.SMTPPort <= 0 {
		return nil, fmt.Errorf("smtp port must be positive")
	}

	return &SMTPMailer{
		host:     cfg.SMTPHost,
		port:     cfg.SMTPPort,
		username: cfg.SMTPUsername,
		password: cfg.SMTPPassword,
		from:     cfg.SMTPFrom,
	}, nil
}

func (m *SMTPMailer) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	subject := "Something.ca - Password Recovery :O"
	textBody := fmt.Sprintf("Hiiii! Here you go, reset your password:\r\n\r\n%s\r\n\r\nIf you did not request this, you can ignore this email.\r\n", resetURL)
	htmlBody := fmt.Sprintf(`<html><body><p>Hiiii! Here you go, reset your password:</p><p><a href="%s">%s</a></p><p>If you did not request this, you can ignore this email.</p></body></html>`, resetURL, resetURL)

	boundary := fmt.Sprintf("boundary-%d", time.Now().UnixNano())
	msg := strings.Join([]string{
		"From: " + m.from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		fmt.Sprintf(`Content-Type: multipart/alternative; boundary="%s"`, boundary),
		"",
		"--" + boundary,
		`Content-Type: text/plain; charset="UTF-8"`,
		"",
		textBody,
		"--" + boundary,
		`Content-Type: text/html; charset="UTF-8"`,
		"",
		htmlBody,
		"--" + boundary + "--",
		"",
	}, "\r\n")

	addr := fmt.Sprintf("%s:%d", m.host, m.port)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial smtp: %w", err)
	}

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer client.Close()

	if err := client.StartTLS(&tls.Config{
		ServerName: m.host,
		MinVersion: tls.VersionTLS12,
	}); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}

	if err := client.Auth(loginAuth{username: m.username, password: m.password}); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := client.Mail(m.from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := writer.Write([]byte(msg)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write smtp message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close smtp message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtp quit: %w", err)
	}
	return nil
}

type loginAuth struct {
	username string
	password string
}

func (a loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}

	prompt := strings.TrimSpace(strings.ToLower(string(fromServer)))
	switch prompt {
	case "username:", base64.StdEncoding.EncodeToString([]byte("Username:")):
		return []byte(a.username), nil
	case "password:", base64.StdEncoding.EncodeToString([]byte("Password:")):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected login auth prompt: %q", string(fromServer))
	}
}
