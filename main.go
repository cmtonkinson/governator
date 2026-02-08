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
	"github.com/cmtonkinson/governator/internal/dag"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/repo"
	"github.com/cmtonkinson/governator/internal/run"
	"github.com/cmtonkinson/governator/internal/status"
	"github.com/cmtonkinson/governator/internal/supervisor"
	"github.com/cmtonkinson/governator/internal/supervisorlock"
	"github.com/cmtonkinson/governator/internal/tui"
)

const usage = `governator - AI-powered task orchestration engine

USAGE:
    governator [global options] <command> [command options]

GLOBAL OPTIONS:
    -h, --help       Show this help message
    -v, --verbose    Enable verbose output for debugging
    -V, --version    Print version and build information

COMMANDS:
    init             Bootstrap a new governator workspace in the current repository
    start            Start the unified supervisor to plan, triage, and execute work
    plan             Alias for 'start'
    execute          Alias for 'start'
    status           Display current supervisor and task status
    why              Show the most recent supervisor log lines
    dag              Display task dependency graph (DAG)
    stop             Stop the running supervisor gracefully
    restart          Stop and restart the current supervisor phase
    reset            Stop supervisor and clear all state (nuclear option)
    tail             Stream agent output logs in real-time

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
	version := globalFlags.Bool("V", false, "")
	versionLong := globalFlags.Bool("version", false, "")

	if len(os.Args) < 2 {
		globalFlags.Usage()
		os.Exit(2)
	}

	// Parse global flags
	args := os.Args[1:]
	for len(args) > 0 && (args[0] == "-v" || args[0] == "--verbose" || args[0] == "-V" || args[0] == "--version") {
		if args[0] == "-v" {
			*verbose = true
		} else if args[0] == "--verbose" {
			*verboseLong = true
		} else if args[0] == "-V" {
			*version = true
		} else if args[0] == "--version" {
			*versionLong = true
		}
		args = args[1:]
	}

	if *version || *versionLong {
		runVersion()
		return
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
	case "start":
		runStart(commandArgs)
	case "plan":
		runPlan(commandArgs)
	case "execute":
		runExecute(commandArgs)
	case "status":
		runStatus(commandArgs)
	case "why":
		runWhy(commandArgs)
	case "dag":
		runDAG(commandArgs)
	case "stop":
		runStop(commandArgs)
	case "restart":
		runRestart(commandArgs)
	case "reset":
		runReset(commandArgs)
	case "tail":
		runTail(commandArgs)
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

	// Configuration override flags
	agent := flags.String("a", "", "")
	agentLong := flags.String("agent", "", "")
	concurrency := flags.Int("c", 0, "")
	concurrencyLong := flags.Int("concurrency", 0, "")
	reasoningEffort := flags.String("r", "", "")
	reasoningEffortLong := flags.String("reasoning-effort", "", "")
	branch := flags.String("b", "", "")
	branchLong := flags.String("branch", "", "")
	timeout := flags.Int("t", 0, "")
	timeoutLong := flags.Int("timeout", 0, "")

	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator init [options]

DESCRIPTION:
    Initialize a new governator workspace in the current git repository.
    Creates the _governator/ directory structure with default configuration,
    seeds the planning index, and commits the initialization.

OPTIONS:
    -a, --agent <cli>             Set default worker CLI (codex, claude, gemini)
    -c, --concurrency <n>         Set global and default role concurrency limit
    -r, --reasoning-effort <lvl>  Set default reasoning effort (low, medium, high)
    -b, --branch <name>           Set base branch name (default: main)
    -t, --timeout <seconds>       Set worker timeout in seconds (default: 900)
    -h, --help                    Show this help message
`)
	}
	flags.Parse(args)

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "governator init: unexpected arguments\n\n")
		flags.Usage()
		os.Exit(2)
	}

	// Resolve flag values
	agentValue := *agent
	if *agentLong != "" {
		agentValue = *agentLong
	}
	concurrencyValue := *concurrency
	if *concurrencyLong != 0 {
		concurrencyValue = *concurrencyLong
	}
	reasoningEffortValue := *reasoningEffort
	if *reasoningEffortLong != "" {
		reasoningEffortValue = *reasoningEffortLong
	}
	branchValue := *branch
	if *branchLong != "" {
		branchValue = *branchLong
	}
	timeoutValue := *timeout
	if *timeoutLong != 0 {
		timeoutValue = *timeoutLong
	}

	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	// Create the layout with default config
	if err := config.InitFullLayout(repoRoot, config.InitOptions{Verbose: verbose}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	// Apply flag overrides to config if any were provided
	if agentValue != "" || concurrencyValue != 0 || reasoningEffortValue != "" || branchValue != "" || timeoutValue != 0 {
		if err := config.ApplyInitOverrides(repoRoot, config.InitOverrides{
			Agent:           agentValue,
			Concurrency:     concurrencyValue,
			ReasoningEffort: reasoningEffortValue,
			Branch:          branchValue,
			Timeout:         timeoutValue,
		}); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
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

// launchSupervisor launches the unified supervisor in the background.
func launchSupervisor(commandArg string) {
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	if _, running, err := supervisor.AnyRunning(repoRoot); err != nil {
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

	logPath := supervisor.LogPath(repoRoot)
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

	cmd := exec.Command(os.Args[0], commandArg, "--supervisor")
	cmd.Dir = repoRoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	pid := cmd.Process.Pid
	state := supervisor.SupervisorStateInfo{
		Phase:          "start",
		PID:            pid,
		State:          supervisor.SupervisorStateRunning,
		StartedAt:      time.Now().UTC(),
		LastTransition: time.Now().UTC(),
		LogPath:        logPath,
	}
	if err := supervisor.SaveState(repoRoot, state); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if err := cmd.Process.Release(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Printf("start supervisor started (pid %d)\n", pid)
}

func runStart(args []string) {
	flags := flag.NewFlagSet("start", flag.ExitOnError)
	supervisorMode := flags.Bool("supervisor", false, "")
	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator start

DESCRIPTION:
    Start the unified supervisor in the background.
    The supervisor plans, triages, executes, and auto-replans on ADR drift.
    Returns immediately after spawning the supervisor process.

    Use 'governator status' to monitor progress and 'governator tail' to stream logs.

OPTIONS:
    -h, --help    Show this help message
`)
	}
	flags.Parse(args)

	if *supervisorMode {
		runStartSupervisor()
		return
	}

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "governator start: unexpected arguments\n\n")
		flags.Usage()
		os.Exit(2)
	}

	launchSupervisor("start")
}

func runStartSupervisor() {
	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	if err := run.RunUnifiedSupervisor(repoRoot, run.UnifiedSupervisorOptions{Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runPlan(args []string) {
	runStart(args)
}

func runExecute(args []string) {
	runStart(args)
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
	if phase, running, err := supervisor.AnyRunning(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	} else if !running {
		fmt.Fprintln(os.Stderr, "no supervisor running")
		os.Exit(1)
	} else {
		_ = phase
		if err := run.StopUnifiedSupervisor(repoRoot, run.UnifiedSupervisorStopOptions{StopWorker: stopWorker}); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		fmt.Println("start supervisor stopped")
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
    Stop the current supervisor and immediately restart unified orchestration.
    If no supervisor is running, starts the unified supervisor.
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
	phase, running, err := supervisor.AnyRunning(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if !running {
		runStart(nil)
		return
	}
	_ = phase
	if err := run.StopUnifiedSupervisor(repoRoot, run.UnifiedSupervisorStopOptions{StopWorker: stopWorker}); err != nil && !errors.Is(err, supervisor.ErrSupervisorNotRunning) {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	runStart(nil)
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
	phase, running, err := supervisor.AnyRunning(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if running {
		_ = phase
		if err := run.StopUnifiedSupervisor(repoRoot, run.UnifiedSupervisorStopOptions{StopWorker: stopWorker}); err != nil && !errors.Is(err, supervisor.ErrSupervisorNotRunning) {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	}
	if _, runningAfter, err := supervisor.AnyRunning(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	} else if runningAfter {
		fmt.Fprintln(os.Stderr, "supervisor still running; retry reset after it exits")
		os.Exit(1)
	}
	if err := supervisor.ClearState(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if err := supervisorlock.Remove(repoRoot, supervisor.SupervisorLockName); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Println("supervisor reset")
}

// ensureNoSupervisorLocks blocks supervisor startup when any lock is held.
func ensureNoSupervisorLocks(repoRoot string) error {
	if held, err := supervisorlock.Held(repoRoot, supervisor.SupervisorLockName); err != nil {
		return err
	} else if held {
		return errors.New("start supervisor lock already held; use governator stop or governator reset first")
	}
	return nil
}

func runStatus(args []string) {
	flags := flag.NewFlagSet("status", flag.ExitOnError)
	interactive := flags.Bool("interactive", false, "Enable interactive mode with live updates")
	interactiveShort := flags.Bool("i", false, "")
	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator status [options]

DESCRIPTION:
    Display current supervisor status and task progress.
    Default mode shows a static snapshot; interactive mode provides live updates.

OPTIONS:
    -i, --interactive    Enable interactive mode with live task updates
    -h, --help           Show this help message
`)
	}
	flags.Parse(args)

	interactiveMode := *interactive || *interactiveShort

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

	if interactiveMode {
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

func runDAG(args []string) {
	flags := flag.NewFlagSet("dag", flag.ExitOnError)
	interactive := flags.Bool("interactive", false, "Enable interactive mode (not yet implemented)")
	interactiveShort := flags.Bool("i", false, "")
	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator dag [options]

DESCRIPTION:
    Display the task dependency graph (DAG).
    Shows dependencies (what a task needs) and blocks (what depends on this task).
    Tasks are ordered by execution order, making it easy to see parallelization opportunities.

OPTIONS:
    -i, --interactive    Enable interactive mode with navigation (not yet implemented)
    -h, --help           Show this help message
`)
	}
	flags.Parse(args)

	interactiveMode := *interactive || *interactiveShort

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "governator dag: unexpected arguments\n\n")
		flags.Usage()
		os.Exit(2)
	}

	if interactiveMode {
		fmt.Fprintln(os.Stderr, "interactive mode not yet implemented")
		os.Exit(1)
	}

	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	idx, err := index.Load(indexPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	dagSummary := dag.GetSummary(idx)
	fmt.Println(dagSummary.String())
}

func runWhy(args []string) {
	flags := flag.NewFlagSet("why", flag.ExitOnError)
	supervisorLinesShort := flags.Int("s", 20, "")
	supervisorLinesLong := flags.Int("supervisor-lines", 20, "")
	taskLinesShort := flags.Int("t", 20, "")
	taskLinesLong := flags.Int("task-lines", 20, "")
	flags.Usage = func() {
		fmt.Fprint(os.Stderr, `USAGE:
    governator why [-s <lines>] [-t <lines>]

DESCRIPTION:
    Print recent supervisor log lines plus recent worker stdout lines for each
    blocked or failed task.

OPTIONS:
    -s, --supervisor-lines <lines>    Supervisor trailing lines (default: 20)
    -t, --task-lines <lines>          Per-task trailing lines (default: 20)
    -h, --help                        Show this help message
`)
	}
	flags.Parse(args)

	supervisorLines := *supervisorLinesShort
	if *supervisorLinesLong != 20 {
		supervisorLines = *supervisorLinesLong
	}
	taskLines := *taskLinesShort
	if *taskLinesLong != 20 {
		taskLines = *taskLinesLong
	}

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "governator why: unexpected arguments\n\n")
		flags.Usage()
		os.Exit(2)
	}
	if supervisorLines <= 0 {
		fmt.Fprintln(os.Stderr, "governator why: -s/--supervisor-lines must be a positive integer")
		os.Exit(2)
	}
	if taskLines <= 0 {
		fmt.Fprintln(os.Stderr, "governator why: -t/--task-lines must be a positive integer")
		os.Exit(2)
	}

	repoRoot, err := repo.DiscoverRootFromCWD()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	logPath := supervisor.LogPath(repoRoot)
	supervisorPID := 0
	supervisorState := "unknown"
	state, ok, err := supervisor.LoadState(repoRoot)
	if err == nil && ok && strings.TrimSpace(state.LogPath) != "" {
		logPath = state.LogPath
	}
	if err == nil && ok {
		supervisorPID = state.PID
		if strings.TrimSpace(string(state.State)) != "" {
			supervisorState = string(state.State)
		}
	}

	lastLines, err := readLastLines(logPath, supervisorLines)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	colorizeHeaders := shouldColorizeOutput(os.Stdout)
	fmt.Fprintln(os.Stdout, formatWhySupervisorHeader(supervisorPID, supervisorState, supervisorLines, colorizeHeaders))
	if len(lastLines) > 0 {
		fmt.Fprintln(os.Stdout, strings.Join(lastLines, "\n"))
	}

	sections, err := collectWhyTaskSections(repoRoot, taskLines, state, ok)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	for _, section := range sections {
		fmt.Fprintln(os.Stdout)
		if section.logPath == "" {
			fmt.Fprintln(os.Stdout, formatWhyTaskMissingHeader(section.taskID, section.kind, colorizeHeaders))
			continue
		}
		displayPath := section.logPath
		if relPath, relErr := filepath.Rel(repoRoot, section.logPath); relErr == nil {
			displayPath = filepath.ToSlash(relPath)
		}
		fmt.Fprintln(os.Stdout, formatWhyTaskHeader(section.taskID, section.kind, taskLines, displayPath, colorizeHeaders))
		if len(section.lines) > 0 {
			fmt.Fprintln(os.Stdout, strings.Join(section.lines, "\n"))
		}
	}
}

const (
	ansiReset = "\x1b[0m"
	ansiDim   = "\x1b[2m"
	ansiRed   = "\x1b[31m"
)

// shouldColorizeOutput reports whether ANSI styling should be emitted.
func shouldColorizeOutput(out *os.File) bool {
	if out == nil || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := out.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// formatWhySupervisorHeader renders the supervisor section header.
func formatWhySupervisorHeader(pid int, state string, lines int, colorize bool) string {
	return formatWhyHeader(
		fmt.Sprintf("=== Supervisor %d (%s) last %d lines ===", pid, state, lines),
		state,
		colorize,
	)
}

// formatWhyTaskHeader renders a task section header with log path context.
func formatWhyTaskHeader(taskID string, kind string, lines int, logPath string, colorize bool) string {
	return formatWhyHeader(
		fmt.Sprintf("=== Task %s (%s) last %d lines from %s ===", taskID, kind, lines, logPath),
		kind,
		colorize,
	)
}

// formatWhyTaskMissingHeader renders a task section header when no log is found.
func formatWhyTaskMissingHeader(taskID string, kind string, colorize bool) string {
	return formatWhyHeader(
		fmt.Sprintf("=== Task %s (%s) no worker stdout log found ===", taskID, kind),
		kind,
		colorize,
	)
}

// formatWhyHeader applies subtle header styling and failure highlighting.
func formatWhyHeader(header string, state string, colorize bool) string {
	if !colorize {
		return header
	}
	styledState := state
	if strings.EqualFold(strings.TrimSpace(state), "failed") {
		styledState = ansiRed + state + ansiReset + ansiDim
	}
	stateToken := "(" + state + ")"
	styledToken := "(" + styledState + ")"
	styledHeader := strings.Replace(header, stateToken, styledToken, 1)
	return ansiDim + styledHeader + ansiReset
}

// readLastLines returns up to maxLines trailing lines from path.
func readLastLines(path string, maxLines int) ([]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("log path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read log %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lines := make([]string, 0, maxLines)
	for scanner.Scan() {
		line := scanner.Text()
		if len(lines) < maxLines {
			lines = append(lines, line)
			continue
		}
		copy(lines, lines[1:])
		lines[len(lines)-1] = line
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan log %s: %w", path, err)
	}
	return lines, nil
}

// whyTaskSection captures a per-task output section for governator why.
type whyTaskSection struct {
	taskID  string
	kind    string
	logPath string
	lines   []string
}

// collectWhyTaskSections returns per-task sections for blocked/failed execution tasks.
func collectWhyTaskSections(repoRoot string, taskLines int, supervisorState supervisor.SupervisorStateInfo, hasSupervisorState bool) ([]whyTaskSection, error) {
	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	idx, err := index.Load(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("load task index: %w", err)
	}

	sections := make([]whyTaskSection, 0)
	for _, task := range idx.Tasks {
		kind := whyTaskKind(task, supervisorState, hasSupervisorState)
		if kind == "" {
			continue
		}

		logPath, err := latestTaskStdoutLog(repoRoot, task.ID)
		if err != nil {
			return nil, err
		}

		section := whyTaskSection{
			taskID:  task.ID,
			kind:    kind,
			logPath: logPath,
		}
		if logPath != "" {
			lines, err := readLastLines(logPath, taskLines)
			if err != nil {
				return nil, err
			}
			section.lines = lines
		}
		sections = append(sections, section)
	}
	return sections, nil
}

// whyTaskKind classifies tasks that should show a task section in governator why.
func whyTaskKind(task index.Task, supervisorState supervisor.SupervisorStateInfo, hasSupervisorState bool) string {
	if task.State == index.TaskStateBlocked {
		return "blocked"
	}
	if task.Attempts.Failed > 0 {
		return "failed"
	}
	if task.Kind == index.TaskKindPlanning &&
		hasSupervisorState &&
		supervisorState.State == supervisor.SupervisorStateFailed &&
		strings.TrimSpace(supervisorState.StepID) == "plan" {
		return "failed"
	}
	return ""
}

// latestTaskStdoutLog returns the most recently modified stdout log for a task.
func latestTaskStdoutLog(repoRoot string, taskID string) (string, error) {
	if strings.TrimSpace(taskID) == "" {
		return "", nil
	}
	taskStateDir := filepath.Join(
		repoRoot,
		"_governator",
		"_local-state",
		"task-"+taskID,
		"_governator",
		"_local-state",
	)
	entries, err := os.ReadDir(taskStateDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read task local state %s: %w", taskStateDir, err)
	}

	latestPath := ""
	var latestTime time.Time
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(taskStateDir, entry.Name(), "stdout.log")
		info, err := os.Stat(candidate)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("stat task stdout log %s: %w", candidate, err)
		}
		if latestPath == "" || info.ModTime().After(latestTime) || (info.ModTime().Equal(latestTime) && candidate > latestPath) {
			latestPath = candidate
			latestTime = info.ModTime()
		}
	}
	return latestPath, nil
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
    Automatically exits when all agents complete. Type q then Enter, or press Ctrl+C, to stop.

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
	go handleTailQuitInput(ctx, os.Stdin, os.Stderr, cancel)

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

// handleTailQuitInput listens for quit keys during tail streaming.
func handleTailQuitInput(ctx context.Context, in io.Reader, out io.Writer, cancel context.CancelFunc) {
	reader := bufio.NewReader(in)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		key, err := reader.ReadByte()
		if err != nil {
			return
		}
		if key == 'q' || key == 'Q' {
			fmt.Fprintln(out, "\nquit requested")
			cancel()
			return
		}
	}
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
