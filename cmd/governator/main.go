// Command governator provides the CLI entrypoint for Governator v2.
package main

import (
	"fmt"
	"os"

	"github.com/cmtonkinson/governator/internal/plan"
	"github.com/cmtonkinson/governator/internal/repo"
	"github.com/cmtonkinson/governator/internal/run"
)

const usageLine = "usage: governator <init|plan|run|status|version>"

func main() {
	if len(os.Args) < 2 {
		emitUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "plan":
		runPlan()
	case "run":
		runRun()
	default:
		emitUsage()
		os.Exit(2)
	}
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
	if _, err := run.Run(repoRoot, run.Options{Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func emitUsage() {
	fmt.Fprintln(os.Stderr, usageLine)
}
