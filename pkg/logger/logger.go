package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// New returns a service-scoped logger. Set LOG_FORMAT=json for machine-readable
// logs (Loki/Promtail); default is console (human-readable).
func New(service string) zerolog.Logger {
	out := os.Stdout
	if strings.EqualFold(strings.TrimSpace(os.Getenv("LOG_FORMAT")), "json") {
		return zerolog.New(out).
			With().
			Timestamp().
			Str("service", service).
			Logger()
	}
	output := zerolog.ConsoleWriter{Out: out, TimeFormat: time.RFC3339}
	return zerolog.New(output).
		With().
		Timestamp().
		Str("service", service).
		Logger()
}
