package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// New creates a configured zerolog.Logger.
// If pretty is true, uses a human-readable console writer (for development).
// Otherwise, outputs structured JSON (for production).
func New(level string, pretty bool) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	var w io.Writer = os.Stdout
	if pretty {
		w = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	return zerolog.New(w).
		Level(lvl).
		With().
		Timestamp().
		Logger()
}
