package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tienlao/agregator/pkg/config"
)

// Sender sends plain-text UTF-8 email via SMTP (STARTTLS on 587, implicit TLS on 465).
type Sender struct {
	host     string
	port     string
	smtpUser string
	smtpPass string
	from     string
	tlsMode  string // from SMTP_TLS: implicit | starttls | off | empty (auto from port)
}

func NewSenderFromEnv() *Sender {
	return &Sender{
		host:     strings.TrimSpace(config.GetEnv("SMTP_HOST", "")),
		port:     strings.TrimSpace(config.GetEnv("SMTP_PORT", "587")),
		smtpUser: strings.TrimSpace(config.GetEnv("SMTP_USER", "")),
		smtpPass: strings.TrimSpace(config.GetEnv("SMTP_PASSWORD", "")),
		from:     strings.TrimSpace(config.GetEnv("SMTP_FROM", "")),
		tlsMode:  strings.TrimSpace(config.GetEnv("SMTP_TLS", "")),
	}
}

func (s *Sender) Enabled() bool {
	return s.host != "" && s.from != "" && s.smtpUser != "" && s.smtpPass != ""
}

// SendPlain sends one message to all recipients (To header lists them).
func (s *Sender) SendPlain(ctx context.Context, to []string, subjectLine, body string) error {
	if !s.Enabled() {
		return fmt.Errorf("smtp not configured")
	}
	if len(to) == 0 {
		return nil
	}
	sendTimeout := durationFromEnv("SMTP_SEND_TIMEOUT", 45*time.Second)
	opCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	return s.sendPlainOnce(opCtx, to, subjectLine, body)
}

func (s *Sender) sendPlainOnce(ctx context.Context, to []string, subjectLine, body string) error {
	port := strings.TrimSpace(s.port)
	if port == "" {
		port = "587"
	}
	host := s.host
	addr := net.JoinHostPort(host, port)
	subject := encodeRFC2047Word(subjectLine)
	msg := buildRFC822(s.from, to, subject, body)
	auth := smtp.PlainAuth("", s.smtpUser, s.smtpPass, host)

	ioDeadline := opIOTimeout(ctx, durationFromEnv("SMTP_SEND_TIMEOUT", 45*time.Second))
	tlsCfg := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}

	implicit := tlsImplicit(port, s.tlsMode)
	dialTimeout := durationFromEnv("SMTP_DIAL_TIMEOUT", 12*time.Second)
	dialer := &net.Dialer{Timeout: dialTimeout}

	var clientConn net.Conn
	var err error
	if implicit {
		raw, derr := dialer.DialContext(ctx, "tcp", addr)
		if derr != nil {
			return fmt.Errorf("smtp dial %s: %w", addr, derr)
		}
		wrapped := &deadlineConn{Conn: raw, deadline: ioDeadline}
		tlsConn := tls.Client(wrapped, tlsCfg)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = tlsConn.Close()
			return fmt.Errorf("smtp tls handshake: %w", err)
		}
		clientConn = tlsConn
	} else {
		raw, derr := dialer.DialContext(ctx, "tcp", addr)
		if derr != nil {
			return fmt.Errorf("smtp dial %s: %w", addr, derr)
		}
		clientConn = &deadlineConn{Conn: raw, deadline: ioDeadline}
	}

	c, err := smtp.NewClient(clientConn, host)
	if err != nil {
		_ = clientConn.Close()
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer c.Close()

	if !implicit {
		switch {
		case plainOnly(s.tlsMode):
			// local relay / Mailpit: no TLS
		case forceStartTLS(s.tlsMode):
			if err := c.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("smtp starttls: %w", err)
			}
		default:
			if ok, _ := c.Extension("STARTTLS"); ok {
				if err := c.StartTLS(tlsCfg); err != nil {
					return fmt.Errorf("smtp starttls: %w", err)
				}
			}
		}
	}
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := c.Mail(s.from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp rcpt %s: %w", rcpt, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}
	// Не вызываем Quit: на части серверов он долго ждёт ответа; письмо уже принято после закрытия DATA.
	return nil
}

// deadlineConn wraps a net.Conn so each Read/Write respects the absolute I/O deadline (SMTP без отдельного context на каждую операцию).
type deadlineConn struct {
	net.Conn
	deadline time.Time
}

func (d *deadlineConn) Read(b []byte) (int, error) {
	if !d.deadline.IsZero() {
		_ = d.Conn.SetReadDeadline(d.deadline)
	}
	return d.Conn.Read(b)
}

func (d *deadlineConn) Write(b []byte) (int, error) {
	if !d.deadline.IsZero() {
		_ = d.Conn.SetWriteDeadline(d.deadline)
	}
	return d.Conn.Write(b)
}

func opIOTimeout(ctx context.Context, fallback time.Duration) time.Time {
	if t, ok := ctx.Deadline(); ok {
		return t
	}
	return time.Now().Add(fallback)
}

func durationFromEnv(key string, def time.Duration) time.Duration {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func tlsImplicit(port, tlsEnv string) bool {
	mode := strings.ToLower(strings.TrimSpace(tlsEnv))
	switch mode {
	case "implicit", "smtps", "ssl":
		return true
	case "starttls", "tls", "off", "none", "plain":
		return false
	}
	p, err := strconv.Atoi(strings.TrimSpace(port))
	return err == nil && p == 465
}

func forceStartTLS(tlsEnv string) bool {
	mode := strings.ToLower(strings.TrimSpace(tlsEnv))
	return mode == "starttls" || mode == "tls"
}

func plainOnly(tlsEnv string) bool {
	mode := strings.ToLower(strings.TrimSpace(tlsEnv))
	return mode == "off" || mode == "none" || mode == "plain"
}

func encodeRFC2047Word(s string) string {
	return fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(s)))
}

func buildRFC822(from string, to []string, subject, body string) []byte {
	headers := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n",
		from,
		strings.Join(to, ", "),
		subject,
	)
	return []byte(headers + body + "\r\n")
}
