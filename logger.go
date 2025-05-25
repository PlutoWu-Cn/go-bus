package bus

import (
	"fmt"
	"log"
	"os"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger defines the interface for logging within the event bus
type Logger interface {
	// Debug logs a debug message
	Debug(msg string, args ...interface{})
	// Info logs an info message
	Info(msg string, args ...interface{})
	// Warn logs a warning message
	Warn(msg string, args ...interface{})
	// Error logs an error message
	Error(msg string, args ...interface{})
	// SetLevel sets the minimum log level
	SetLevel(level LogLevel)
	// GetLevel returns the current log level
	GetLevel() LogLevel
}

// DefaultLogger is the default logger implementation using Go's standard log package
type DefaultLogger struct {
	level  LogLevel
	logger *log.Logger
}

// NewDefaultLogger creates a new default logger instance
func NewDefaultLogger() *DefaultLogger {
	return &DefaultLogger{
		level:  LogLevelInfo,
		logger: log.New(os.Stdout, "[EventBus] ", log.LstdFlags|log.Lshortfile),
	}
}

// NewDefaultLoggerWithOutput creates a new default logger with custom output
func NewDefaultLoggerWithOutput(output *os.File, prefix string) *DefaultLogger {
	return &DefaultLogger{
		level:  LogLevelInfo,
		logger: log.New(output, prefix, log.LstdFlags|log.Lshortfile),
	}
}

// Debug logs a debug message
func (l *DefaultLogger) Debug(msg string, args ...interface{}) {
	if l.level <= LogLevelDebug {
		l.logger.Printf("[%s] %s", LogLevelDebug.String(), fmt.Sprintf(msg, args...))
	}
}

// Info logs an info message
func (l *DefaultLogger) Info(msg string, args ...interface{}) {
	if l.level <= LogLevelInfo {
		l.logger.Printf("[%s] %s", LogLevelInfo.String(), fmt.Sprintf(msg, args...))
	}
}

// Warn logs a warning message
func (l *DefaultLogger) Warn(msg string, args ...interface{}) {
	if l.level <= LogLevelWarn {
		l.logger.Printf("[%s] %s", LogLevelWarn.String(), fmt.Sprintf(msg, args...))
	}
}

// Error logs an error message
func (l *DefaultLogger) Error(msg string, args ...interface{}) {
	if l.level <= LogLevelError {
		l.logger.Printf("[%s] %s", LogLevelError.String(), fmt.Sprintf(msg, args...))
	}
}

// SetLevel sets the minimum log level
func (l *DefaultLogger) SetLevel(level LogLevel) {
	l.level = level
}

// GetLevel returns the current log level
func (l *DefaultLogger) GetLevel() LogLevel {
	return l.level
}

// NoOpLogger is a logger that does nothing (useful for disabling logging)
type NoOpLogger struct{}

// NewNoOpLogger creates a new no-op logger
func NewNoOpLogger() *NoOpLogger {
	return &NoOpLogger{}
}

// Debug does nothing
func (l *NoOpLogger) Debug(msg string, args ...interface{}) {}

// Info does nothing
func (l *NoOpLogger) Info(msg string, args ...interface{}) {}

// Warn does nothing
func (l *NoOpLogger) Warn(msg string, args ...interface{}) {}

// Error does nothing
func (l *NoOpLogger) Error(msg string, args ...interface{}) {}

// SetLevel does nothing
func (l *NoOpLogger) SetLevel(level LogLevel) {}

// GetLevel returns LogLevelError (highest level to disable all logging)
func (l *NoOpLogger) GetLevel() LogLevel {
	return LogLevelError + 1
}
