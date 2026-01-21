// Package run provides helpers for governator run orchestration safeguards.
package run

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cmtonkinson/governator/internal/audit"
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/runlock"
)

const (
	guardLocalStateDirName = "_governator/_local_state"
	guardFileName          = "run.guard"
	guardFileMode          = 0o644
	guardDirMode           = 0o755

	guardEventTaskID = "run"
	guardEventRole   = "governator"

	guardStatusAllowed = "allowed"
	guardStatusBlocked = "blocked"

	guardReasonAllowed  = "allowed"
	guardReasonLockHeld = "lock_held"
	guardReasonCooldown = "cooldown"
)

// SelfRunGuard enforces cool-down and locking when auto-rerun behavior is enabled.
type SelfRunGuard struct {
	repoRoot string
	cfg      config.AutoRerunConfig
	auditor  *audit.Logger
	now      func() time.Time
	lock     *runlock.Lock
}

// GuardOutcome describes the result of evaluating the self-run guard.
type GuardOutcome struct {
	Allowed bool
	Reason  string
	Message string
}

func newSelfRunGuard(repoRoot string, cfg config.AutoRerunConfig, auditor *audit.Logger) *SelfRunGuard {
	return &SelfRunGuard{
		repoRoot: repoRoot,
		cfg:      cfg,
		auditor:  auditor,
		now:      time.Now,
	}
}

// EnsureAllowed acquires the run lock and validates the cooldown window before running.
func (guard *SelfRunGuard) EnsureAllowed() (GuardOutcome, error) {
	lock, err := runlock.Acquire(guard.repoRoot)
	if err != nil {
		if errors.Is(err, runlock.ErrLockHeld) {
			outcome := GuardOutcome{
				Allowed: false,
				Reason:  guardReasonLockHeld,
				Message: err.Error(),
			}
			guard.logDecision(outcome, 0, outcome.Message)
			return outcome, nil
		}
		return GuardOutcome{}, fmt.Errorf("acquire run guard lock: %w", err)
	}
	guard.lock = lock

	now := guard.now().UTC()
	lastRun, err := guard.readLastRun()
	if err != nil {
		_ = guard.lock.Release()
		guard.lock = nil
		return GuardOutcome{}, fmt.Errorf("read run guard timestamp: %w", err)
	}

	elapsed := time.Duration(0)
	if !lastRun.IsZero() {
		elapsed = now.Sub(lastRun)
		if elapsed < 0 {
			elapsed = 0
		}
	}

	cooldown := time.Duration(guard.cfg.CooldownSeconds) * time.Second
	if cooldown > 0 && !lastRun.IsZero() && elapsed < cooldown {
		remaining := cooldown - elapsed
		message := fmt.Sprintf("run guard cooldown active; try again in %.0fs", remaining.Seconds())
		outcome := GuardOutcome{
			Allowed: false,
			Reason:  guardReasonCooldown,
			Message: message,
		}
		guard.logDecision(outcome, elapsed, message)
		_ = guard.lock.Release()
		guard.lock = nil
		return outcome, nil
	}

	if err := guard.writeLastRun(now); err != nil {
		_ = guard.lock.Release()
		guard.lock = nil
		return GuardOutcome{}, fmt.Errorf("update run guard timestamp: %w", err)
	}

	outcome := GuardOutcome{
		Allowed: true,
		Reason:  guardReasonAllowed,
	}
	guard.logDecision(outcome, elapsed, "")
	return outcome, nil
}

// Release unlocks the run lock held by the guard.
func (guard *SelfRunGuard) Release() error {
	if guard == nil || guard.lock == nil {
		return nil
	}
	err := guard.lock.Release()
	guard.lock = nil
	return err
}

func (guard *SelfRunGuard) guardFilePath() string {
	return filepath.Join(guard.repoRoot, guardLocalStateDirName, guardFileName)
}

func (guard *SelfRunGuard) readLastRun() (time.Time, error) {
	path := guard.guardFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("read guard file %s: %w", path, err)
	}
	ts := strings.TrimSpace(string(data))
	if ts == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse guard timestamp: %w", err)
	}
	return parsed.UTC(), nil
}

func (guard *SelfRunGuard) writeLastRun(value time.Time) error {
	path := guard.guardFilePath()
	if err := os.MkdirAll(filepath.Dir(path), guardDirMode); err != nil {
		return fmt.Errorf("create guard directory: %w", err)
	}
	payload := []byte(value.UTC().Format(time.RFC3339))
	if err := os.WriteFile(path, payload, guardFileMode); err != nil {
		return fmt.Errorf("write guard file: %w", err)
	}
	return nil
}

func (guard *SelfRunGuard) logDecision(outcome GuardOutcome, elapsed time.Duration, detail string) {
	if guard.auditor == nil {
		return
	}

	fields := []audit.Field{
		{Key: "status", Value: guardStatusForOutcome(outcome)},
		{Key: "reason", Value: outcome.Reason},
		{Key: "cooldown_seconds", Value: strconv.Itoa(guard.cfg.CooldownSeconds)},
	}
	if detail != "" {
		fields = append(fields, audit.Field{Key: "details", Value: detail})
	}
	if elapsed > 0 {
		fields = append(fields, audit.Field{Key: "seconds_since_last_run", Value: fmt.Sprintf("%.0f", elapsed.Seconds())})
	}

	_ = guard.auditor.Log(audit.Entry{
		TaskID: guardEventTaskID,
		Role:   guardEventRole,
		Event:  audit.EventRunGuard,
		Fields: fields,
	})
}

func guardStatusForOutcome(outcome GuardOutcome) string {
	if outcome.Allowed {
		return guardStatusAllowed
	}
	return guardStatusBlocked
}
