package logger

import (
	"fmt"
	"io"
	"strings"

	"github.com/rs/zerolog"
)

func New(output io.Writer, level string) (zerolog.Logger, error) {
	parsedLevel, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		return zerolog.Logger{}, fmt.Errorf("parse log level: %w", err)
	}
	return zerolog.New(output).
		Level(parsedLevel).
		With().
		Timestamp().
		Str("service", "llmatch-api").
		Logger(), nil
}
