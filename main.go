// Command governator provides the CLI entrypoint for Governator v2.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cmtonkinson/governator/internal/buildinfo"
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/repo"
	"github.com/cmtonkinson/governator/internal/run"
	"github.com/cmtonkinson/governator/internal/status"
	"github.com/cmtonkinson/governator/internal/supervisor"
	"github.com/cmtonkinson/governator/internal/tui"
	"github.com/cmtonkinson/governator/internal/supervisorlock"
)

const usageLine = "usage: governator [-v|--verbose] <init|plan|execute|run|status|stop|restart|reset|tail|version>"

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
		runExecute(args[1:])
	case "run":
		runRun()
	case "status":
		runStatus(args[1:])
	case "stop":
		runStop(args[1:])
	case "restart":
		runRestart(args[1:])
	case "reset":
		runReset(args[1:])
	case "tail":
		runTail(args[1:])
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
	if _, running, err := supervisor.AnySupervisorRunning(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	} else if running {
		fmt.Fprintln(os.Stderr, "supervisor already running; use governator stop or governator reset first")
		os.Exit(1)
	}
	if err := ensureNoSupervisorLocks(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
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

func runExecute(args []string) {
	if len(args) > 0 && args[0] == "--supervisor" {
		runExecuteSupervisor()
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
	if _, running, err := supervisor.AnySupervisorRunning(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	} else if running {
		fmt.Fprintln(os.Stderr, "supervisor already running; use governator stop or governator reset first")
		os.Exit(1)
	}
	if err := ensureNoSupervisorLocks(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	logPath := supervisor.ExecutionLogPath(repoRoot)
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

	cmd := exec.Command(os.Args[0], "execute", "--supervisor")
	cmd.Dir = repoRoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	pid := cmd.Process.Pid
	state := supervisor.ExecutionSupervisorState{
		Phase:          "execution",
		PID:            pid,
		State:          supervisor.SupervisorStateRunning,
		StartedAt:      time.Now().UTC(),
		LastTransition: time.Now().UTC(),
		LogPath:        logPath,
	}
	if err := supervisor.SaveExecutionState(repoRoot, state); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if err := cmd.Process.Release(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Printf("execution supervisor started (pid %d)\n", pid)
}

func runExecuteSupervisor() {
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}
	if err := run.RunExecutionSupervisor(repoRoot, run.ExecutionSupervisorOptions{Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
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
	if phase, running, err := supervisor.AnySupervisorRunning(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	} else if !running {
		fmt.Fprintln(os.Stderr, "no supervisor running")
		os.Exit(1)
	} else if phase == "planning" {
		if err := run.StopPlanningSupervisor(repoRoot, run.PlanningSupervisorStopOptions{StopWorker: stopWorker}); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		fmt.Println("planning supervisor stopped")
	} else {
		if err := run.StopExecutionSupervisor(repoRoot, run.ExecutionSupervisorStopOptions{StopWorker: stopWorker}); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		fmt.Println("execution supervisor stopped")
	}
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
	phase, running, err := supervisor.AnySupervisorRunning(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if !running {
		runPlan(nil)
		return
	}
	if phase == "planning" {
		if err := run.StopPlanningSupervisor(repoRoot, run.PlanningSupervisorStopOptions{StopWorker: stopWorker}); err != nil && !errors.Is(err, supervisor.ErrPlanningSupervisorNotRunning) {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		runPlan(nil)
		return
	}
	if err := run.StopExecutionSupervisor(repoRoot, run.ExecutionSupervisorStopOptions{StopWorker: stopWorker}); err != nil && !errors.Is(err, supervisor.ErrExecutionSupervisorNotRunning) {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	runExecute(nil)
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
	phase, running, err := supervisor.AnySupervisorRunning(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if running && phase == "planning" {
		if err := run.StopPlanningSupervisor(repoRoot, run.PlanningSupervisorStopOptions{StopWorker: stopWorker}); err != nil && !errors.Is(err, supervisor.ErrPlanningSupervisorNotRunning) {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	}
	if running && phase == "execution" {
		if err := run.StopExecutionSupervisor(repoRoot, run.ExecutionSupervisorStopOptions{StopWorker: stopWorker}); err != nil && !errors.Is(err, supervisor.ErrExecutionSupervisorNotRunning) {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	}
	if _, runningAfter, err := supervisor.AnySupervisorRunning(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	} else if runningAfter {
		fmt.Fprintln(os.Stderr, "supervisor still running; retry reset after it exits")
		os.Exit(1)
	}
	if err := supervisor.ClearPlanningState(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if err := supervisor.ClearExecutionState(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if err := supervisorlock.Remove(repoRoot, supervisor.PlanningSupervisorLockName); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if err := supervisorlock.Remove(repoRoot, supervisor.ExecutionSupervisorLockName); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Println("supervisor reset")
}

// ensureNoSupervisorLocks blocks supervisor startup when any lock is held.
func ensureNoSupervisorLocks(repoRoot string) error {
	if held, err := supervisorlock.Held(repoRoot, supervisor.PlanningSupervisorLockName); err != nil {
		return err
	} else if held {
		return errors.New("planning supervisor lock already held; use governator stop or governator reset first")
	}
	if held, err := supervisorlock.Held(repoRoot, supervisor.ExecutionSupervisorLockName); err != nil {
		return err
	} else if held {
		return errors.New("execution supervisor lock already held; use governator stop or governator reset first")
	}
	return nil
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

func runStatus(args []string) {
	// Parse flags
	watchMode := false
	for _, arg := range args {
		if arg == "--watch" || arg == "-w" {
			watchMode = true
		}
	}

	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		emitUsage()
		os.Exit(2)
	}

	if watchMode {
		// Interactive mode
		if err := tui.Run(repoRoot); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}

	// Static mode (existing implementation)
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

func runTail(args []string) {
	// Parse flags (--stdout to include stdout, --both for both streams)
	includeStdout := false
	for _, arg := range args {
		if arg == "--stdout" {
			includeStdout = true
		} else if arg == "--both" {
			includeStdout = true
		} else {
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", arg)
			emitUsage()
			os.Exit(2)
		}
	}

	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	store, err := inflight.NewStore(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	inFlight, err := store.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if inFlight == nil || len(inFlight) == 0 {
		fmt.Println("no active agents")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// Periodically check if agents are still active
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				currentInFlight, err := store.Load()
				if err != nil {
					continue
				}
				if currentInFlight == nil || len(currentInFlight) == 0 {
					fmt.Fprintln(os.Stderr, "\nall agents completed")
					cancel()
					return
				}
			}
		}
	}()

	var wg sync.WaitGroup
	for _, entry := range inFlight {
		stderrPath := filepath.Join(entry.WorkerStateDir, "stderr.log")
		wg.Add(1)
		go func(id, path string) {
			defer wg.Done()
			tailLogFile(ctx, id, "stderr", path, os.Stdout)
		}(entry.ID, stderrPath)

		if includeStdout {
			stdoutPath := filepath.Join(entry.WorkerStateDir, "stdout.log")
			wg.Add(1)
			go func(id, path string) {
				defer wg.Done()
				tailLogFile(ctx, id, "stdout", path, os.Stdout)
			}(entry.ID, stdoutPath)
		}
	}

	wg.Wait()
}

func tailLogFile(ctx context.Context, taskID string, stream string, path string, out io.Writer) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	cmd := exec.CommandContext(ctx, "tail", "-f", "-n", "+1", path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}

	if err := cmd.Start(); err != nil {
		return
	}

	scanner := bufio.NewScanner(stdout)
	prefix := fmt.Sprintf("[%s:%s]", taskID, stream)
	for scanner.Scan() {
		fmt.Fprintf(out, "%s %s\n", prefix, scanner.Text())
	}

	cmd.Wait()
}

func emitUsage() {
	fmt.Fprintln(os.Stderr, usageLine)
}
