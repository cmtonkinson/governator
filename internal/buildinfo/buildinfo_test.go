package buildinfo

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		commit   string
		builtAt  string
		expected string
	}{
		{
			name:     "default values",
			version:  "dev",
			commit:   "unknown",
			builtAt:  "unknown",
			expected: "version=dev commit=unknown built_at=unknown",
		},
		{
			name:     "release values",
			version:  "1.2.3",
			commit:   "8d3f2a1",
			builtAt:  "2025-02-14T09:30:00Z",
			expected: "version=1.2.3 commit=8d3f2a1 built_at=2025-02-14T09:30:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original values
			origVersion := Version
			origCommit := Commit
			origBuiltAt := BuiltAt

			// Set test values
			Version = tt.version
			Commit = tt.commit
			BuiltAt = tt.builtAt

			// Test
			result := String()
			if result != tt.expected {
				t.Errorf("String() = %q, want %q", result, tt.expected)
			}

			// Restore original values
			Version = origVersion
			Commit = origCommit
			BuiltAt = origBuiltAt
		})
	}
}

func TestStringFormat(t *testing.T) {
	result := String()

	// Check that it contains the expected format
	if !strings.Contains(result, "version=") {
		t.Error("String() should contain 'version='")
	}
	if !strings.Contains(result, "commit=") {
		t.Error("String() should contain 'commit='")
	}
	if !strings.Contains(result, "built_at=") {
		t.Error("String() should contain 'built_at='")
	}

	// Check that spaces separate the components
	parts := strings.Split(result, " ")
	if len(parts) != 3 {
		t.Errorf("String() should have 3 space-separated parts, got %d: %q", len(parts), result)
	}
}
