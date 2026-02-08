// Package run provides backward-compatible execution supervisor entrypoints.
package run

import (
	"io"
	"time"
)

// ExecutionSupervisorOptions configures the legacy execution supervisor entrypoint.
// Deprecated: use UnifiedSupervisorOptions with RunUnifiedSupervisor.
type ExecutionSupervisorOptions struct {
	Stdout       io.Writer
	Stderr       io.Writer
	PollInterval time.Duration
	LogPath      string
}

// RunExecutionSupervisor runs the unified supervisor loop.
// Deprecated: use RunUnifiedSupervisor.
func RunExecutionSupervisor(repoRoot string, opts ExecutionSupervisorOptions) error {
	return RunUnifiedSupervisor(repoRoot, UnifiedSupervisorOptions{
		Stdout:       opts.Stdout,
		Stderr:       opts.Stderr,
		PollInterval: opts.PollInterval,
		LogPath:      opts.LogPath,
	})
}
