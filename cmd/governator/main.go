// Command governator provides the CLI entrypoint for Governator v2.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/cmtonkinson/governator/internal/buildinfo"
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/phase"
	"github.com/cmtonkinson/governator/internal/plan"
	"github.com/cmtonkinson/governator/internal/repo"
	"github.com/cmtonkinson/governator/internal/run"
	"github.com/cmtonkinson/governator/internal/status"
)

const usageLine = "usage: governator [-v|--verbose] <init|plan|run|status|version>"

func main() {
	verbose := false
	args := os.Args[1:]

flagLoop:
	for len(args) > 0 {
		switch args[0] {
		case "-v", "--verbose":
			verbose = true
			args = args[1:]
		default:
			break flagLoop
		}
	}

	if len(args) == 0 {
		emitUsage()
		os.Exit(2)
	}

	switch args[0] {
	case "init":
		runInit(verbose)
	case "plan":
		runPlan()
	case "run":
		runRun()
	case "status":
		runStatus()
	case "version":
		runVersion()
	default:
		emitUsage()
		os.Exit(2)
	}
}

func runInit(verbose bool) {
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	if err := config.InitFullLayout(repoRoot, config.InitOptions{Verbose: verbose}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Println("init ok")
}

func runPlan() {
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	if _, err := plan.Run(repoRoot, plan.Options{Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runRun() {
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	if err := executeRunWithPhase(repoRoot, run.Options{Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runStatus() {
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	summary, err := status.GetSummary(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Println(summary.String())
}

func runVersion() {
	fmt.Println(buildinfo.String())
}

func emitUsage() {
	fmt.Fprintln(os.Stderr, usageLine)
}

func executeRunWithPhase(repoRoot string, opts run.Options) error {
	store := phase.NewStore(repoRoot)
	state, err := store.Load()
	if err != nil {
		return fmt.Errorf("load phase state: %w", err)
	}
	if state.Current == phase.PhaseComplete {
		fmt.Fprintf(opts.Stdout, "phase %d complete\n", state.Current.Number())
		return nil
	}
	if state.Agent.PID != 0 && state.Agent.FinishedAt.IsZero() {
		return fmt.Errorf("phase %d (%s) already running (pid %d started %s)", state.Current.Number(), state.Current, state.Agent.PID, state.Agent.StartedAt.Format(time.RFC3339))
	}

	validations, err := phase.ValidatePrerequisites(repoRoot, state.Current)
	if err != nil {
		return fmt.Errorf("validate phase prerequisites: %w", err)
	}
	state.ArtifactValidations = validations
	state.Notes = fmt.Sprintf("pre-run validation for phase %d", state.Current.Number())
	if err := store.Save(state); err != nil {
		return fmt.Errorf("save phase state: %w", err)
	}

	failed := collectFailedValidations(validations)
	if len(failed) > 0 {
		for _, validation := range failed {
			fmt.Fprintf(opts.Stderr, "phase gate: %s (%s)\n", validation.Name, validation.Message)
		}
		return fmt.Errorf("phase %d (%s) is blocked by missing artifacts", state.Current.Number(), state.Current)
	}

	fmt.Fprintf(opts.Stdout, "phase %d running\n", state.Current.Number())
	now := time.Now()
	state.Agent = phase.AgentMetadata{
		PID:       os.Getpid(),
		StartedAt: now,
	}
	if err := store.Save(state); err != nil {
		return fmt.Errorf("save phase state: %w", err)
	}

	result, runErr := run.Run(repoRoot, opts)
	state.Agent.FinishedAt = time.Now()
	if runErr != nil {
		state.Notes = fmt.Sprintf("run failure: %v", runErr)
		if saveErr := store.Save(state); saveErr != nil {
			return fmt.Errorf("run failed: %v (phase state save failed: %w)", runErr, saveErr)
		}
		return runErr
	}

	state.LastCompleted = state.Current
	state.Current = state.Current.Next()
	if result.Message != "" {
		state.Notes = result.Message
	} else {
		state.Notes = fmt.Sprintf("phase %d completed", state.LastCompleted.Number())
	}
	if err := store.Save(state); err != nil {
		return fmt.Errorf("save phase state: %w", err)
	}

	return nil
}

func collectFailedValidations(validations []phase.ArtifactValidation) []phase.ArtifactValidation {
	var failed []phase.ArtifactValidation
	for _, validation := range validations {
		if !validation.Valid {
			failed = append(failed, validation)
		}
	}
	return failed
}
