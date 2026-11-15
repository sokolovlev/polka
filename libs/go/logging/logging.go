// Package logging builds the structured JSON logger every Go service uses
// (spec 9.4: JSON logs with trace_id, shipped to Loki).
package logging

import (
	"io"
	"log/slog"
	"os"
)

// New returns a JSON slog logger writing to stdout with a "service" attribute.
func New(service, level string) *slog.Logger {
	return NewWithWriter(os.Stdout, service, level)
}

// NewWithWriter is New with an explicit writer (for tests).
func NewWithWriter(w io.Writer, service, level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: l})
	return slog.New(h).With(slog.String("service", service))
}
