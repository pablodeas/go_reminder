// Package logger provides a structured, colorized, leveled logger
// with HTTP middleware and automatic caller information.
package logger

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Level defines log severity.
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
	FATAL
)

var levelNames = map[Level]string{
	DEBUG: "DEBUG",
	INFO:  "INFO ",
	WARN:  "WARN ",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

var levelColors = map[Level]string{
	DEBUG: "\033[36m", // cyan
	INFO:  "\033[32m", // green
	WARN:  "\033[33m", // yellow
	ERROR: "\033[31m", // red
	FATAL: "\033[35m", // magenta
}

const colorReset = "\033[0m"
const colorGray = "\033[90m"
const colorBold = "\033[1m"

// Logger is a named, leveled logger instance.
type Logger struct {
	mu       sync.Mutex
	out      io.Writer
	minLevel Level
	color    bool
	prefix   string
}

// std is the default package-level logger.
var std = &Logger{
	out:      os.Stderr,
	minLevel: DEBUG,
	color:    isTerminal(),
}

func isTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// SetLevel sets the minimum log level for the default logger.
func SetLevel(l Level) { std.minLevel = l }

// SetOutput redirects the default logger's output (disables color).
func SetOutput(w io.Writer) { std.out = w; std.color = false }

// New returns a child Logger with a named prefix that inherits the
// global output and level settings.
func New(prefix string) *Logger {
	return &Logger{
		out:      std.out,
		minLevel: std.minLevel,
		color:    std.color,
		prefix:   prefix,
	}
}

// ── core emit ─────────────────────────────────────────────────

func (l *Logger) emit(level Level, msg string, args ...interface{}) {
	if level < l.minLevel {
		return
	}

	now := time.Now().Format("2006-01-02 15:04:05.000")

	// Caller: skip emit + public shim
	_, file, line, ok := runtime.Caller(2)
	caller := "???"
	if ok {
		parts := strings.Split(file, "/")
		n := len(parts)
		switch {
		case n >= 3:
			caller = fmt.Sprintf("%s/%s:%d", parts[n-2], parts[n-1], line)
		case n == 2:
			caller = fmt.Sprintf("%s/%s:%d", parts[0], parts[1], line)
		default:
			caller = fmt.Sprintf("%s:%d", file, line)
		}
	}

	formatted := msg
	if len(args) > 0 {
		formatted = fmt.Sprintf(msg, args...)
	}

	prefix := ""
	if l.prefix != "" {
		prefix = "[" + l.prefix + "] "
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	var out string
	if l.color {
		lc := levelColors[level]
		ln := levelNames[level]
		out = fmt.Sprintf("%s%s%s %s%s%s%s %s%s%s %s%s%s\n",
			colorGray, now, colorReset,
			lc, colorBold, ln, colorReset,
			colorGray, caller, colorReset,
			lc, prefix+formatted, colorReset,
		)
	} else {
		out = fmt.Sprintf("%s [%s] %s %s%s\n",
			now, levelNames[level], caller, prefix, formatted)
	}

	fmt.Fprint(l.out, out)

	if level == FATAL {
		os.Exit(1)
	}
}

// ── Logger methods ─────────────────────────────────────────────

func (l *Logger) Debug(msg string, args ...interface{}) { l.emit(DEBUG, msg, args...) }
func (l *Logger) Info(msg string, args ...interface{})  { l.emit(INFO, msg, args...) }
func (l *Logger) Warn(msg string, args ...interface{})  { l.emit(WARN, msg, args...) }
func (l *Logger) Error(msg string, args ...interface{}) { l.emit(ERROR, msg, args...) }
func (l *Logger) Fatal(msg string, args ...interface{}) { l.emit(FATAL, msg, args...) }

// ── Package-level shortcuts (default logger) ──────────────────

func Debug(msg string, args ...interface{}) { std.emit(DEBUG, msg, args...) }
func Info(msg string, args ...interface{})  { std.emit(INFO, msg, args...) }
func Warn(msg string, args ...interface{})  { std.emit(WARN, msg, args...) }
func Error(msg string, args ...interface{}) { std.emit(ERROR, msg, args...) }
func Fatal(msg string, args ...interface{}) { std.emit(FATAL, msg, args...) }

// ── HTTP Middleware ────────────────────────────────────────────

// Middleware logs every HTTP request with method, path, status and latency.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &captureWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		elapsed := time.Since(start)

		lvl := INFO
		msg := ""
		switch {
		case rw.status >= 500:
			lvl = ERROR
			msg = "⚠ "
		case rw.status >= 400:
			lvl = WARN
			msg = "  "
		default:
			msg = "  "
		}

		std.emit(lvl, "%s%-6s %-40s → %d  %s%s",
			msg, r.Method, r.RequestURI, rw.status, elapsed.Round(time.Millisecond), "")
	})
}

// captureWriter captures the HTTP status code written by a handler.
type captureWriter struct {
	http.ResponseWriter
	status int
}

func (c *captureWriter) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

// ── Standard library bridge ────────────────────────────────────

// BridgeStdLog redirects output from the standard "log" package into
// this logger (INFO level) so all log calls go through one pipe.
func BridgeStdLog() {
	log.SetOutput(&stdBridge{})
	log.SetFlags(0)
}

type stdBridge struct{}

func (b *stdBridge) Write(p []byte) (n int, err error) {
	msg := strings.TrimRight(string(p), "\n\r")
	std.emit(INFO, "%s", msg)
	return len(p), nil
}
