// Package logx provides leveled, file-backed logging for hi.
// Log files rotate daily — each day gets its own file with a date suffix.
package logx

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level represents a log severity level.
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

var levelNames = map[Level]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
}

func (l Level) String() string {
	if s, ok := levelNames[l]; ok {
		return s
	}
	return fmt.Sprintf("LEVEL(%d)", l)
}

// Logger is a leveled logger that writes to a daily-rotated file.
type Logger struct {
	mu       sync.Mutex
	out      io.Writer
	level    Level
	file     *os.File
	basePath string // original path without date (e.g. /tmp/hi.log)
	filePath string // current file with date (e.g. /tmp/hi-2026-06-05.log)
	prefix   string
}

var std = func() *Logger {
	l := &Logger{
		out:    os.Stderr,
		level:  INFO,
		prefix: "[hi]",
	}
	return l
}()

// Init initializes the standard logger with a daily-rotated file output.
// /tmp/hi.log → /tmp/hi-2026-06-05.log
func Init(basePath string, level Level) error {
	std.mu.Lock()
	defer std.mu.Unlock()

	if basePath != "" {
		dir := filepath.Dir(basePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create log directory %s: %w", dir, err)
		}
		std.basePath = basePath
		if err := std.openFileLocked(buildDailyPath(basePath)); err != nil {
			return err
		}
	}

	std.level = level
	return nil
}

// buildDailyPath inserts today's date into the filename.
// hi.log → hi-2026-06-05.log
func buildDailyPath(base string) string {
	ext := filepath.Ext(base)
	prefix := base[:len(base)-len(ext)]
	return prefix + "-" + time.Now().Format("2006-01-02") + ext
}

func (l *Logger) openFileLocked(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file %s: %w", path, err)
	}
	if l.file != nil {
		l.file.Close()
	}
	l.file = f
	l.filePath = path
	l.out = f
	return nil
}

// SetLevel changes the log level at runtime.
func SetLevel(level Level) {
	std.mu.Lock()
	defer std.mu.Unlock()
	std.level = level
}

// LevelStr parses a log level string.
func LevelStr(s string) Level {
	switch s {
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "warn":
		return WARN
	case "error":
		return ERROR
	default:
		return INFO
	}
}

// Close closes the log file if open.
func Close() {
	std.mu.Lock()
	defer std.mu.Unlock()
	if std.file != nil {
		std.file.Close()
		std.file = nil
	}
}

func output(level Level, format string, args ...interface{}) {
	std.mu.Lock()
	defer std.mu.Unlock()

	if level < std.level {
		return
	}

	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s %-5s %s %s\n", std.prefix, level.String(), time.Now().Format("15:04:05"), msg)

	// Check if the date changed — switch to a new file if so.
	if std.basePath != "" {
		wantPath := buildDailyPath(std.basePath)
		if wantPath != std.filePath {
			_ = std.openFileLocked(wantPath)
		}
	}

	if std.file != nil {
		std.file.Write([]byte(line))
	} else {
		os.Stderr.Write([]byte(line))
	}
}

// Debug logs a debug message.
func Debug(format string, args ...interface{}) { output(DEBUG, format, args...) }

// Info logs an info message.
func Info(format string, args ...interface{}) { output(INFO, format, args...) }

// Warn logs a warning message.
func Warn(format string, args ...interface{}) { output(WARN, format, args...) }

// Error logs an error message.
func Error(format string, args ...interface{}) { output(ERROR, format, args...) }

// Logf is a drop-in replacement for log.Printf at INFO level.
func Logf(format string, args ...interface{}) { output(INFO, format, args...) }

// Fatalf logs at ERROR level and exits.
func Fatalf(format string, args ...interface{}) {
	output(ERROR, format, args...)
	os.Exit(1)
}

// FilePath returns the current log file path, or empty if not set.
func FilePath() string {
	std.mu.Lock()
	defer std.mu.Unlock()
	return std.filePath
}
