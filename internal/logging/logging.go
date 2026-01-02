package logging

import (
	"log"
	"log/slog"
	"os"
	"strings"
)

// Config contains logging configuration.
type Config struct {
	Level  string // debug, info, warn, error
	Format string // json, text
}

// Setup configures the default slog logger based on config.
// Returns the configured logger.
func Setup(cfg Config) *slog.Logger {
	level := parseLevel(cfg.Level)

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler

	format := strings.ToLower(cfg.Format)
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		if format != "" && format != "json" {
			log.Printf("WARNING: unrecognized log format %q, defaulting to json", cfg.Format)
		}

		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}

// parseLevel converts a string level to slog.Level.
// Warns and defaults to Info if unrecognized.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		log.Printf("WARNING: unrecognized log level %q, defaulting to info", level)

		return slog.LevelInfo
	}
}
