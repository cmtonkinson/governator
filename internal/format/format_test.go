package format_test

import (
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/format"
)

func TestDurationShort(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero duration", 0, "0s"},
		{"less than a second", 500 * time.Millisecond, "0s"},
		{"exact second", 1 * time.Second, "1s"},
		{"multiple seconds", 30 * time.Second, "30s"},
		{"exact minute", 1 * time.Minute, "1m0s"},
		{"minutes and seconds", 2*time.Minute + 30*time.Second, "2m30s"},
		{"exact hour", 1 * time.Hour, "1h0m0s"},
		{"hours, minutes, seconds", 1*time.Hour + 2*time.Minute + 3*time.Second, "1h2m3s"},
		{"large duration", 25*time.Hour + 45*time.Minute + 15*time.Second, "25h45m15s"},
		{"negative duration", -10 * time.Second, "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := format.DurationShort(tt.duration)
			if got != tt.expected {
				t.Errorf("DurationShort(%v) = %q, want %q", tt.duration, got, tt.expected)
			}
		})
	}
}

func TestTokens(t *testing.T) {
	tests := []struct {
		name     string
		n        int
		expected string
	}{
		{"zero", 0, "0"},
		{"single digit", 5, "5"},
		{"two digits", 42, "42"},
		{"three digits", 123, "123"},
		{"four digits", 1234, "1,234"},
		{"five digits", 12345, "12,345"},
		{"six digits", 123456, "123,456"},
		{"seven digits", 1234567, "1,234,567"},
		{"negative number", -100, "0"}, // Should be handled as 0 or error, current implementation returns 0.
		{"large number", 1234567890, "1,234,567,890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := format.Tokens(tt.n)
			if got != tt.expected {
				t.Errorf("Tokens(%d) = %q, want %q", tt.n, got, tt.expected)
			}
		})
	}
}

func TestPID(t *testing.T) {
	tests := []struct {
		name     string
		pid      int
		expected string
	}{
		{"positive PID", 12345, "12345"},
		{"zero PID", 0, ""},
		{"negative PID", -1, ""},
		{"large PID", 987654321, "987654321"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := format.PID(tt.pid)
			if got != tt.expected {
				t.Errorf("PID(%d) = %q, want %q", tt.pid, got, tt.expected)
			}
		})
	}
}
