package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/runlock"
)

func TestSelfRunGuardBlocksUnderCooldown(t *testing.T) {
	repo := t.TempDir()
	cfg := config.AutoRerunConfig{
		Enabled:         true,
		CooldownSeconds: 5,
	}

	baseTime := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	guard := newSelfRunGuard(repo, cfg, nil)
	guard.now = func() time.Time { return baseTime }

	outcome, err := guard.EnsureAllowed()
	if err != nil {
		t.Fatalf("ensure allowed: %v", err)
	}
	if !outcome.Allowed {
		t.Fatalf("expected guard allowed, got %+v", outcome)
	}
	if outcome.Reason != guardReasonAllowed {
		t.Fatalf("unexpected reason: %s", outcome.Reason)
	}
	if err := guard.Release(); err != nil {
		t.Fatalf("release guard: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(repo, guardLocalStateDirName, guardFileName))
	if err != nil {
		t.Fatalf("read guard file: %v", err)
	}
	if got := strings.TrimSpace(string(content)); got != baseTime.Format(time.RFC3339) {
		t.Fatalf("guard timestamp = %q, want %q", got, baseTime.Format(time.RFC3339))
	}

	guard2 := newSelfRunGuard(repo, cfg, nil)
	guard2.now = func() time.Time { return baseTime.Add(1 * time.Second) }
	outcome2, err := guard2.EnsureAllowed()
	if err != nil {
		t.Fatalf("ensure allowed second run: %v", err)
	}
	if outcome2.Allowed {
		t.Fatalf("expected guard blocked, got %+v", outcome2)
	}
	if outcome2.Reason != guardReasonCooldown {
		t.Fatalf("unexpected reason: %s", outcome2.Reason)
	}
	if !strings.Contains(outcome2.Message, "try again") {
		t.Fatalf("unexpected message: %q", outcome2.Message)
	}
}

func TestSelfRunGuardBlocksWhenLockHeld(t *testing.T) {
	repo := t.TempDir()
	cfg := config.AutoRerunConfig{
		Enabled:         true,
		CooldownSeconds: 1,
	}

	lock, err := runlock.Acquire(repo)
	if err != nil {
		t.Fatalf("acquire run lock: %v", err)
	}
	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil {
			t.Fatalf("release lock: %v", releaseErr)
		}
	}()

	guard := newSelfRunGuard(repo, cfg, nil)
	outcome, err := guard.EnsureAllowed()
	if err != nil {
		t.Fatalf("ensure allowed: %v", err)
	}
	if outcome.Allowed {
		t.Fatalf("expected guard blocked by lock, got %+v", outcome)
	}
	if outcome.Reason != guardReasonLockHeld {
		t.Fatalf("unexpected reason: %s", outcome.Reason)
	}
	if !strings.Contains(outcome.Message, "already held") {
		t.Fatalf("unexpected message: %q", outcome.Message)
	}
}
