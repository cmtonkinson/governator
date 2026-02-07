# Governator Test Infrastructure

## Test Worker

The `test-worker.sh` script provides deterministic LLM worker behavior for E2E/integration testing without making actual API calls.

### How It Works

1. Reads a prompt file (stitched from role, contract, task, etc.)
2. Matches prompt content against regex patterns in `testdata/fixtures/worker-actions.yaml`
3. Executes file actions (write, modify, delete) for ALL matching rules
4. Logs matched rules to stderr for debugging

### Dependencies

- `yq` (YAML processor): Install via `brew install yq` (macOS) or https://github.com/mikefarah/yq

### Usage in Tests

Configure your test to use the test worker instead of real LLM:

```go
func TestMyFeature(t *testing.T) {
    // Point to test worker script (use absolute path or ensure it's in PATH)
    testWorkerPath, _ := filepath.Abs("tests/e2e/test-worker.sh")

    // Configure governator to use test worker
    cfg := &config.Config{
        Workers: config.WorkersConfig{
            Default: []string{testWorkerPath, "{prompt_path}"},
        },
        // ... other config
    }

    // Run your test...
}
```

Alternatively, set via environment variable:

```bash
GOVERNATOR_TEST_FIXTURES=/path/to/custom-fixtures.yaml go test ./...
```

### Fixture Format

Fixtures map regex patterns to file actions:

```yaml
rules:
  - pattern: 'your.*regex.*here'
    actions:
      # Write a new file (or overwrite existing)
      - write:
          path: 'relative/path/from/repo/root.md'
          content: |
            File contents here
            Multi-line supported

      # Modify existing file
      - modify:
          path: 'existing/file.go'
          operation: 'append'  # or 'prepend' or 'replace'
          content: 'content to add'
          # For replace operation:
          match: 'regex.*pattern'  # required only for replace

      # Delete a file
      - delete:
          path: 'file/to/remove.txt'
```

### Adding New Test Scenarios

1. Identify the prompt pattern you need to handle
2. Add a new rule to `tests/e2e/testdata/fixtures/worker-actions.yaml`
3. Define the file actions that simulate LLM worker output
4. Run your test

### Debugging

The test worker logs to stderr:

```
[test-worker] Matched rule 0: planning.*epic
[test-worker] Writing file: _governator/planning/epics.md
[test-worker] Completed successfully (matched 1 rule(s))
```

If no rules match, you'll see:

```
[test-worker] WARNING: No rules matched prompt
[test-worker] Prompt preview: <first 3 lines of prompt>
```

### Example: E2E Test

```go
func TestPlanningWorkflow(t *testing.T) {
    repo := testrepos.TempRepo(t)

    // Use test worker
    testWorkerPath, _ := filepath.Abs("tests/e2e/test-worker.sh")
    cfg := &config.Config{
        Workers: config.WorkersConfig{
            Default: []string{testWorkerPath, "{prompt_path}"},
        },
    }

    // Initialize governator state
    err := config.InitializeStateDirectory(repo.Dir, cfg)
    require.NoError(t, err)

    // Create a planning task
    taskPath := filepath.Join(repo.Dir, "_governator/tasks/plan-epic.md")
    err = os.WriteFile(taskPath, []byte("Create an epic for planning feature X"), 0644)
    require.NoError(t, err)

    // Run orchestrator (or specific component under test)
    // ...

    // Assert that test worker created expected output
    epicsPath := filepath.Join(repo.Dir, "_governator/planning/epics.md")
    require.FileExists(t, epicsPath)

    content, _ := os.ReadFile(epicsPath)
    require.Contains(t, string(content), "Epic 1")
}
```

## Tips

- **Multiple matches are intentional**: If a prompt matches multiple patterns, ALL rules execute. Use specific patterns to avoid conflicts.
- **Paths are relative to repo root**: Actions execute from the repository root directory.
- **Keep fixtures simple**: Focus on minimal side effects needed to validate routing/orchestration, not business logic.
- **Static content only**: No templating/variables in this version—fixtures are deterministic.
