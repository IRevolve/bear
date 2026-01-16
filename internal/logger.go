package internal

import (
	"io"
	"log/slog"
	"os"
)

var (
	// Logger is the global logger instance
	Logger *slog.Logger
	// Verbose controls debug output
	Verbose bool
)

func init() {
	// Default: Info level, no debug output
	SetupLogger(false)
}

// SetupLogger configures the global logger
func SetupLogger(verbose bool) {
	Verbose = verbose

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	Logger = slog.New(handler)
}

// SetupSilentLogger disables all logging (for tests)
func SetupSilentLogger() {
	handler := slog.NewTextHandler(io.Discard, nil)
	Logger = slog.New(handler)
}

// Debug logs a debug message (only shown with --verbose)
func Debug(msg string, args ...any) {
	Logger.Debug(msg, args...)
}

// Info logs an info message
func Info(msg string, args ...any) {
	Logger.Info(msg, args...)
}

// Warn logs a warning message
func Warn(msg string, args ...any) {
	Logger.Warn(msg, args...)
}

// Error logs an error message
func Error(msg string, args ...any) {
	Logger.Error(msg, args...)
}
