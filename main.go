// Command governator provides the CLI entrypoint for Governator v2.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cmtonkinson/governator/internal/buildinfo"
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/repo"
	"github.com/cmtonkinson/governator/internal/run"
	"github.com/cmtonkinson/governator/internal/status"
)

const usageLine = "usage: governator [-v|--verbose] <init|run|status|version>"

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
	if _, err := run.Run(repoRoot, run.Options{Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
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
