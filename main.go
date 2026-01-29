// Command governator provides the CLI entrypoint for Governator v2.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cmtonkinson/governator/internal/buildinfo"
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/repo"
	"github.com/cmtonkinson/governator/internal/run"
	"github.com/cmtonkinson/governator/internal/status"
	"github.com/cmtonkinson/governator/internal/supervisor"
)

const usageLine = "usage: governator [-v|--verbose] <init|plan|execute|run|status|stop|restart|reset|version>"

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
		runPlan(args[1:])
	case "execute":
		runExecute()
	case "run":
		runRun()
	case "status":
		runStatus()
	case "stop":
		runStop(args[1:])
	case "restart":
		runRestart(args[1:])
	case "reset":
		runReset(args[1:])
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
	if err := run.SeedPlanningIndex(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if err := commitInit(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Println("init ok")
}

func commitInit(repoRoot string) error {
	addCmd := exec.Command("git", "add", "--", "_governator")
	addCmd.Dir = repoRoot
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add _governator failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	diffCmd := exec.Command("git", "diff", "--cached", "--quiet", "--", "_governator")
	diffCmd.Dir = repoRoot
	if err := diffCmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			commitCmd := exec.Command("git", "commit", "-m", "Governator initialized")
			commitCmd.Dir = repoRoot
			commitCmd.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME=Governator CLI",
				"GIT_AUTHOR_EMAIL=governator@localhost",
				"GIT_COMMITTER_NAME=Governator CLI",
				"GIT_COMMITTER_EMAIL=governator@localhost",
			)
			if out, err := commitCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("git commit failed: %s: %w", strings.TrimSpace(string(out)), err)
			}
			return nil
		}
		return fmt.Errorf("git diff --cached failed: %w", err)
	}
	return nil
}

func runRun() {
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "Warning: governator run is deprecated. Use `governator plan` followed by `governator execute`.")
	if _, err := run.Run(repoRoot, run.Options{Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runPlan(args []string) {
	if len(args) > 0 && args[0] == "--supervisor" {
		runPlanSupervisor()
		return
	}
	if len(args) > 0 {
		emitUsage()
		os.Exit(2)
	}
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	if _, running, err := supervisor.PlanningSupervisorRunning(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	} else if running {
		fmt.Fprintln(os.Stderr, "planning supervisor already running; use governator stop or governator reset first")
		os.Exit(1)
	}

	logPath := supervisor.PlanningLogPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	defer logFile.Close()

	cmd := exec.Command(os.Args[0], "plan", "--supervisor")
	cmd.Dir = repoRoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	pid := cmd.Process.Pid
	state := supervisor.PlanningSupervisorState{
		Phase:          "planning",
		PID:            pid,
		State:          supervisor.SupervisorStateRunning,
		StartedAt:      time.Now().UTC(),
		LastTransition: time.Now().UTC(),
		LogPath:        logPath,
	}
	if err := supervisor.SavePlanningState(repoRoot, state); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if err := cmd.Process.Release(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Printf("planning supervisor started (pid %d)\n", pid)
}

func runPlanSupervisor() {
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	if err := run.RunPlanningSupervisor(repoRoot, run.PlanningSupervisorOptions{Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runExecute() {
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	if _, err := run.Execute(repoRoot, run.Options{Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runStop(args []string) {
	stopWorker, err := parseWorkerFlag(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	if err := run.StopPlanningSupervisor(repoRoot, run.PlanningSupervisorStopOptions{StopWorker: stopWorker}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Println("planning supervisor stopped")
}

func runRestart(args []string) {
	stopWorker, err := parseWorkerFlag(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	if err := run.StopPlanningSupervisor(repoRoot, run.PlanningSupervisorStopOptions{StopWorker: stopWorker}); err != nil && !errors.Is(err, supervisor.ErrPlanningSupervisorNotRunning) {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	runPlan(nil)
}

func runReset(args []string) {
	stopWorker, err := parseWorkerFlag(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	if err := run.StopPlanningSupervisor(repoRoot, run.PlanningSupervisorStopOptions{StopWorker: stopWorker}); err != nil && !errors.Is(err, supervisor.ErrPlanningSupervisorNotRunning) {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if err := supervisor.ClearPlanningState(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Println("planning supervisor reset")
}

func parseWorkerFlag(args []string) (bool, error) {
	stopWorker := false
	for _, arg := range args {
		if arg == "--worker" || arg == "-w" {
			stopWorker = true
			continue
		}
		return false, fmt.Errorf("unknown flag %q", arg)
	}
	return stopWorker, nil
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
