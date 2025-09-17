package logging

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	LogLevelKey   = "log.level"
	LogFormatKey  = "log.format"
	LogNoColorKey = "log.no_color"
)

// Init sets up the global logger. If sensitive values are provided,
// it wraps the standard output with a redacting writer to mask those values in logs.
func Init(sensitiveValues []string) {
	var queue []string

	levelStr := strings.ToLower(viper.GetString(LogLevelKey))
	level, err := zerolog.ParseLevel(levelStr)
	if err != nil {
		level = zerolog.InfoLevel
		queue = append(queue, fmt.Sprintf("invalid log level %q, using info", levelStr))
	}
	zerolog.SetGlobalLevel(level)

	var output io.Writer = os.Stderr
	logFormat := strings.ToLower(viper.GetString(LogFormatKey))

	if len(sensitiveValues) > 0 {
		output = NewRedactingWriter(output, sensitiveValues)
	}

	if logFormat == "json" {
		log.Logger = zerolog.New(output).With().
			Timestamp().
			Logger()
	} else {
		if logFormat != "console" {
			queue = append(queue, fmt.Sprintf("unknown log format %q, using console", logFormat))
		}
		log.Logger = zerolog.New(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
			w.Out = output
			w.NoColor = viper.GetBool(LogNoColorKey)
			w.TimeFormat = "15:04:05.000"
		})).With().
			Timestamp().
			Logger()
	}

	// now after we set up the logger, we can log any queued messages
	for _, msg := range queue {
		log.Warn().Msg(msg)
	}
}

type RedactingWriter struct {
	underlying io.Writer
	sensitive  []string
}

func NewRedactingWriter(underlying io.Writer, sensitive []string) *RedactingWriter {
	return &RedactingWriter{
		underlying: underlying,
		sensitive:  sensitive,
	}
}

func (rw *RedactingWriter) Write(p []byte) (n int, err error) {
	messageBytes := p

	for _, secret := range rw.sensitive {
		if bytes.Contains(messageBytes, []byte(secret)) {
			messageBytes = bytes.ReplaceAll(messageBytes, []byte(secret), []byte("********"))
		}
	}

	return rw.underlying.Write(messageBytes)
}
