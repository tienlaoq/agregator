package mail

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTlsImplicit(t *testing.T) {
	assert.True(t, tlsImplicit("465", ""))
	assert.True(t, tlsImplicit("465", "  "))
	assert.False(t, tlsImplicit("587", ""))
	assert.False(t, tlsImplicit("25", ""))
	assert.True(t, tlsImplicit("587", "implicit"))
	assert.True(t, tlsImplicit("25", "smtps"))
	assert.False(t, tlsImplicit("465", "starttls"))
	assert.False(t, tlsImplicit("465", "off"))
}

func TestForceStartTLS(t *testing.T) {
	assert.True(t, forceStartTLS("starttls"))
	assert.True(t, forceStartTLS("TLS"))
	assert.False(t, forceStartTLS(""))
	assert.False(t, forceStartTLS("implicit"))
}

func TestPlainOnly(t *testing.T) {
	assert.True(t, plainOnly("off"))
	assert.True(t, plainOnly("PLAIN"))
	assert.False(t, plainOnly(""))
	assert.False(t, plainOnly("starttls"))
}

func TestDurationFromEnv(t *testing.T) {
	t.Setenv("SMTP_DIAL_TIMEOUT", "3s")
	assert.Equal(t, 3*time.Second, durationFromEnv("SMTP_DIAL_TIMEOUT", time.Second))
	t.Setenv("SMTP_DIAL_TIMEOUT", "bogus")
	assert.Equal(t, 7*time.Second, durationFromEnv("SMTP_DIAL_TIMEOUT", 7*time.Second))
}

func TestOpIOTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	d := opIOTimeout(ctx, time.Hour)
	assert.WithinDuration(t, time.Now().Add(100*time.Millisecond), d, 50*time.Millisecond)

	d2 := opIOTimeout(context.Background(), 2*time.Second)
	assert.WithinDuration(t, time.Now().Add(2*time.Second), d2, 3*time.Second)
}

func TestSendPlain_connectionRefused(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	host, port, err := net.SplitHostPort(lis.Addr().String())
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	t.Setenv("SMTP_HOST", host)
	t.Setenv("SMTP_PORT", port)
	t.Setenv("SMTP_USER", "u@example.com")
	t.Setenv("SMTP_PASSWORD", "pw")
	t.Setenv("SMTP_FROM", "u@example.com")
	t.Setenv("SMTP_TLS", "off")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s := NewSenderFromEnv()
	err = s.SendPlain(ctx, []string{"to@example.com"}, "subj", "body")
	require.Error(t, err)
}

func TestSendPlain_contextCancelled(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer lis.Close()
	host, port, err := net.SplitHostPort(lis.Addr().String())
	require.NoError(t, err)

	t.Setenv("SMTP_HOST", host)
	t.Setenv("SMTP_PORT", port)
	t.Setenv("SMTP_USER", "u@example.com")
	t.Setenv("SMTP_PASSWORD", "pw")
	t.Setenv("SMTP_FROM", "u@example.com")
	t.Setenv("SMTP_TLS", "off")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := NewSenderFromEnv()
	err = s.SendPlain(ctx, []string{"to@example.com"}, "subj", "body")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
