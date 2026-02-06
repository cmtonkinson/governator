// Command governator provides the CLI entrypoint for Governator v2.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
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

const usage = `governator - AI-powered task orchestration engine

USAGE:
    governator [global options] <command> [command options]

GLOBAL OPTIONS:
    -v, --verbose    Enable verbose output for debugging

COMMANDS:
    init             Bootstrap a new governator workspace in the current repository
    plan             Start the planning supervisor to analyze tasks and generate execution plan
    execute          Start the execution supervisor to run the generated plan
    run              (Deprecated) Run planning and execution in sequence without supervision
    status           Display current supervisor and task status
    stop             Stop the running supervisor gracefully
    restart          Stop and restart the current supervisor phase
    reset            Stop supervisor and clear all state (nuclear option)
    tail             Stream agent output logs in real-time
    version          Print version and build information

Run 'governator <command> -h' for command-specific help.
`

func main() {
	// Global flags
	globalFlags := flag.NewFlagSet("governator", flag.ExitOnError)
	globalFlags.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
	}
	verbose := globalFlags.Bool("v", false, "")
	verboseLong := globalFlags.Bool("verbose", false, "")

	if len(os.Args) < 2 {
		globalFlags.Usage()
		os.Exit(2)
	}

	// Parse global flags
	args := os.Args[1:]
	for len(args) > 0 && (args[0] == "-v" || args[0] == "--verbose") {
		if args[0] == "-v" {
			*verbose = true
		} else {
			*verboseLong = true
		}
		args = args[1:]
	}
	isVerbose := *verbose || *verboseLong

	if len(args) == 0 {
		globalFlags.Usage()
		os.Exit(2)
	}

	// Route to command
	command := args[0]
	commandArgs := args[1:]

	switch command {
	case "init":
		runInit(isVerbose, commandArgs)
	case "plan":
		runPlan(commandArgs)
	case "execute":
		runExecute(commandArgs)
	case "run":
		runRun(commandArgs)
	case "status":
		runStatus(commandArgs)
	case "stop":
		runStop(commandArgs)
	case "restart":
		runRestart(commandArgs)
	case "reset":
		runReset(commandArgs)
	case "tail":
		runTail(commandArgs)
	case "version":
		runVersion()
	case "-h", "--help", "help":
		globalFlags.Usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "governator: unknown command %q\n\n", command)
		globalFlags.Usage()
		os.Exit(2)
	}
}

func runInit(verbose bool, args []string) {
	flags := flag.NewFlagSet("init", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator init

DESCRIPTION:
    Initialize a new governator workspace in the current git repository.
    Creates the _governator/ directory structure with default configuration,
    seeds the planning index, and commits the initialization.

OPTIONS:
    -h, --help    Show this help message
`)
	}
	flags.Parse(args)

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "governator init: unexpected arguments\n\n")
		flags.Usage()
		os.Exit(2)
	}

	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
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

func runRun(args []string) {
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator run

DESCRIPTION:
    [DEPRECATED] Run planning and execution phases sequentially without supervision.
    This command is deprecated; use 'governator plan' followed by 'governator execute'.

OPTIONS:
    -h, --help    Show this help message
`)
	}
	flags.Parse(args)

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "governator run: unexpected arguments\n\n")
		flags.Usage()
		os.Exit(2)
	}

	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "Warning: governator run is deprecated. Use `governator plan` followed by `governator execute`.")
	if _, err := run.Run(repoRoot, run.Options{Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runPlan(args []string) {
	flags := flag.NewFlagSet("plan", flag.ExitOnError)
	supervisorMode := flags.Bool("supervisor", false, "")
	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator plan

DESCRIPTION:
    Start the planning supervisor in the background.
    The supervisor analyzes pending tasks, generates an execution DAG, and prepares
    for execution. Returns immediately after spawning the supervisor process.

    Use 'governator status' to monitor progress and 'governator tail' to stream logs.

OPTIONS:
    -h, --help    Show this help message
`)
	}
	flags.Parse(args)

	if *supervisorMode {
		runPlanSupervisor()
		return
	}

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "governator plan: unexpected arguments\n\n")
		flags.Usage()
		os.Exit(2)
	}

	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
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
		os.Exit(2)
	}
	if err := run.RunPlanningSupervisor(repoRoot, run.PlanningSupervisorOptions{Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runExecute(args []string) {
	flags := flag.NewFlagSet("execute", flag.ExitOnError)
	supervisorMode := flags.Bool("supervisor", false, "")
	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator execute

DESCRIPTION:
    Start the execution supervisor in the background.
    The supervisor processes the execution plan created by 'governator plan',
    dispatching work to agents and managing task orchestration.
    Returns immediately after spawning the supervisor process.

    Use 'governator status' to monitor progress and 'governator tail' to stream logs.

OPTIONS:
    -h, --help    Show this help message
`)
	}
	flags.Parse(args)

	if *supervisorMode {
		runExecuteSupervisor()
		return
	}

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "governator execute: unexpected arguments\n\n")
		flags.Usage()
		os.Exit(2)
	}

	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
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
		os.Exit(2)
	}
	if err := run.RunExecutionSupervisor(repoRoot, run.ExecutionSupervisorOptions{Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runStop(args []string) {
	flags := flag.NewFlagSet("stop", flag.ExitOnError)
	worker := flags.Bool("worker", false, "Also stop running worker agents")
	workerShort := flags.Bool("w", false, "")
	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator stop [options]

DESCRIPTION:
    Stop the currently running supervisor gracefully.
    The supervisor will finish in-flight operations and shut down cleanly.
    State is preserved for restart or inspection.

OPTIONS:
    -w, --worker    Also stop any running worker agents (default: supervisor only)
    -h, --help      Show this help message
`)
	}
	flags.Parse(args)

	stopWorker := *worker || *workerShort

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "governator stop: unexpected arguments\n\n")
		flags.Usage()
		os.Exit(2)
	}

	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
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
	flags := flag.NewFlagSet("restart", flag.ExitOnError)
	worker := flags.Bool("worker", false, "Also stop running worker agents")
	workerShort := flags.Bool("w", false, "")
	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator restart [options]

DESCRIPTION:
    Stop the current supervisor and immediately restart it in the same phase.
    If no supervisor is running, starts the planning phase.
    Useful for picking up configuration changes or recovering from errors.

OPTIONS:
    -w, --worker    Also stop any running worker agents before restart
    -h, --help      Show this help message
`)
	}
	flags.Parse(args)

	stopWorker := *worker || *workerShort

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "governator restart: unexpected arguments\n\n")
		flags.Usage()
		os.Exit(2)
	}

	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
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
	flags := flag.NewFlagSet("reset", flag.ExitOnError)
	worker := flags.Bool("worker", false, "Also stop running worker agents")
	workerShort := flags.Bool("w", false, "")
	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator reset [options]

DESCRIPTION:
    Nuclear option: stop the supervisor and clear all state.
    This removes supervisor state files, clears locks, and prepares for a fresh start.
    Use this when the supervisor is stuck or state is corrupted.

    WARNING: In-progress work may be lost. Use 'governator stop' for graceful shutdown.

OPTIONS:
    -w, --worker    Also stop any running worker agents before reset
    -h, --help      Show this help message
`)
	}
	flags.Parse(args)

	stopWorker := *worker || *workerShort

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "governator reset: unexpected arguments\n\n")
		flags.Usage()
		os.Exit(2)
	}

	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
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

func runStatus(args []string) {
	flags := flag.NewFlagSet("status", flag.ExitOnError)
	watch := flags.Bool("watch", false, "Enable interactive watch mode with live updates")
	watchShort := flags.Bool("w", false, "")
	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator status [options]

DESCRIPTION:
    Display current supervisor status and task progress.
    Default mode shows a static snapshot; watch mode provides live updates.

OPTIONS:
    -w, --watch    Enable interactive watch mode with live task updates
    -h, --help     Show this help message
`)
	}
	flags.Parse(args)

	watchMode := *watch || *watchShort

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "governator status: unexpected arguments\n\n")
		flags.Usage()
		os.Exit(2)
	}

	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
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
	flags := flag.NewFlagSet("tail", flag.ExitOnError)
	stdout := flags.Bool("stdout", false, "Include stdout stream (default: stderr only)")
	both := flags.Bool("both", false, "Include both stdout and stderr streams")
	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator tail [options]

DESCRIPTION:
    Stream real-time output from all active worker agents.
    Each line is prefixed with [task_id:stream] for identification.
    Automatically exits when all agents complete. Press Ctrl+C to stop.

OPTIONS:
    --stdout      Include stdout stream in addition to stderr
    --both        Alias for --stdout (include both stdout and stderr)
    -h, --help    Show this help message
`)
	}
	flags.Parse(args)

	includeStdout := *stdout || *both

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "governator tail: unexpected arguments\n\n")
		flags.Usage()
		os.Exit(2)
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
