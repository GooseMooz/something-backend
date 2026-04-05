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
	textBody := fmt.Sprintf("Hiiii!\r\n\r\nWe got a request to reset your password for Something Matters.\r\n\r\nReset it here:\r\n%s\r\n\r\nIf you didn't request this, you can safely ignore this email.\r\n", resetURL)
	htmlBody := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <body style="margin:0;padding:0;background-color:#ddd0bc;font-family:Arial,sans-serif;color:#2f2a24;">
    <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background-color:#ddd0bc;padding:32px 16px;">
      <tr>
        <td align="center">
          <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="max-width:560px;background-color:#f7f1e8;border:1px solid #cabda9;border-radius:18px;overflow:hidden;">
            <tr>
              <td style="padding:0;">
                <div style="height:10px;background-color:#74ba92;"></div>
              </td>
            </tr>
            <tr>
              <td style="padding:32px 32px 20px 32px;">
                <div style="font-size:13px;letter-spacing:0.12em;text-transform:uppercase;color:#6d6256;font-weight:700;">Something Matters</div>
                <h1 style="margin:14px 0 12px 0;font-size:28px;line-height:1.2;color:#1f3f31;">Password Reset</h1>
                <p style="margin:0 0 14px 0;font-size:16px;line-height:1.6;color:#3e3832;">Hiiii! We got a request to reset your password.</p>
                <p style="margin:0 0 24px 0;font-size:16px;line-height:1.6;color:#3e3832;">Use the button below to choose a new one.</p>
                <table role="presentation" cellspacing="0" cellpadding="0" style="margin:0 0 24px 0;">
                  <tr>
                    <td align="center" bgcolor="#74ba92" style="border-radius:999px;">
                      <a href="%s" style="display:inline-block;padding:14px 24px;font-size:15px;font-weight:700;color:#163126;text-decoration:none;">Reset Password</a>
                    </td>
                  </tr>
                </table>
                <p style="margin:0 0 10px 0;font-size:14px;line-height:1.6;color:#5d554d;">If the button doesn't work, copy and paste this link into your browser:</p>
                <p style="margin:0 0 24px 0;font-size:14px;line-height:1.6;word-break:break-all;"><a href="%s" style="color:#2d6f53;text-decoration:underline;">%s</a></p>
                <div style="padding:16px 18px;background-color:#efe5d8;border-radius:14px;font-size:14px;line-height:1.6;color:#5d554d;">
                  If you didn't request this, you can safely ignore this email. Your password won't change unless you complete the reset.
                </div>
              </td>
            </tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>`, resetURL, resetURL, resetURL)

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
