#!/usr/bin/env bash
set -euo pipefail

# Governator test worker - executes deterministic actions based on prompt patterns
# Usage: test-worker.sh <prompt_file_path>

# Check dependencies
if ! command -v yq &> /dev/null; then
    echo "[test-worker] ERROR: yq is required but not installed" >&2
    echo "[test-worker] Install via: brew install yq (macOS) or https://github.com/mikefarah/yq" >&2
    exit 1
fi

# Read prompt file
if [ $# -lt 1 ]; then
    echo "[test-worker] ERROR: prompt file path required" >&2
    echo "Usage: $0 <prompt_file_path>" >&2
    exit 1
fi

prompt_file="$1"

# If prompt file is relative, try multiple resolution strategies
if [[ "$prompt_file" != /* ]]; then
    # Strategy 1: Use GOVERNATOR_PROMPT_PATH if available (stitched prompt in worker state dir)
    if [ -n "${GOVERNATOR_PROMPT_PATH:-}" ] && [ -f "$GOVERNATOR_PROMPT_PATH" ]; then
        prompt_file="$GOVERNATOR_PROMPT_PATH"
    # Strategy 2: Resolve from worktree dir
    elif [ -n "${GOVERNATOR_WORKTREE_DIR:-}" ]; then
        prompt_file_from_worktree="$GOVERNATOR_WORKTREE_DIR/$prompt_file"
        if [ -f "$prompt_file_from_worktree" ]; then
            prompt_file="$prompt_file_from_worktree"
        else
            # Strategy 3: Go up from worktree to repo root (_local-state/task-planning → repo)
            repo_root="$GOVERNATOR_WORKTREE_DIR/../../.."
            prompt_file_from_repo="$repo_root/$prompt_file"
            if [ -f "$prompt_file_from_repo" ]; then
                prompt_file="$prompt_file_from_repo"
            fi
        fi
    fi
fi

if [ ! -f "$prompt_file" ]; then
    echo "[test-worker] ERROR: prompt file not found: $prompt_file" >&2
    echo "[test-worker] Working directory: $(pwd)" >&2
    echo "[test-worker] GOVERNATOR_WORKTREE_DIR: ${GOVERNATOR_WORKTREE_DIR:-not set}" >&2
    echo "[test-worker] GOVERNATOR_PROMPT_PATH: ${GOVERNATOR_PROMPT_PATH:-not set}" >&2
    exit 1
fi

prompt_content=$(cat "$prompt_file")

# Load fixture config
fixture_file="${GOVERNATOR_TEST_FIXTURES:-test/fixtures/worker-actions.yaml}"
if [ ! -f "$fixture_file" ]; then
    echo "[test-worker] ERROR: fixture file not found: $fixture_file" >&2
    echo "[test-worker] Set GOVERNATOR_TEST_FIXTURES to override default location" >&2
    exit 1
fi

# Find matching rules
rule_count=$(yq eval '.rules | length' "$fixture_file")
matched_rules=()

for ((i=0; i<rule_count; i++)); do
    pattern=$(yq eval ".rules[$i].pattern" "$fixture_file")

    if echo "$prompt_content" | grep -qE "$pattern"; then
        echo "[test-worker] Matched rule $i: $pattern" >&2
        matched_rules+=("$i")
    fi
done

if [ ${#matched_rules[@]} -eq 0 ]; then
    echo "[test-worker] WARNING: No rules matched prompt" >&2
    echo "[test-worker] Prompt preview: $(echo "$prompt_content" | head -n 3)" >&2
    exit 0
fi

# Execute actions for all matched rules
for rule_idx in "${matched_rules[@]}"; do
    action_count=$(yq eval ".rules[$rule_idx].actions | length" "$fixture_file")

    for ((j=0; j<action_count; j++)); do
        action_type=$(yq eval ".rules[$rule_idx].actions[$j] | keys | .[0]" "$fixture_file")

        case "$action_type" in
            write)
                path=$(yq eval ".rules[$rule_idx].actions[$j].write.path" "$fixture_file")
                content=$(yq eval ".rules[$rule_idx].actions[$j].write.content" "$fixture_file")

                echo "[test-worker] Writing file: $path" >&2
                mkdir -p "$(dirname "$path")"
                echo "$content" > "$path"
                ;;

            modify)
                path=$(yq eval ".rules[$rule_idx].actions[$j].modify.path" "$fixture_file")
                operation=$(yq eval ".rules[$rule_idx].actions[$j].modify.operation" "$fixture_file")
                content=$(yq eval ".rules[$rule_idx].actions[$j].modify.content" "$fixture_file")

                if [ ! -f "$path" ]; then
                    echo "[test-worker] ERROR: Cannot modify non-existent file: $path" >&2
                    exit 1
                fi

                case "$operation" in
                    append)
                        echo "[test-worker] Appending to file: $path" >&2
                        echo "$content" >> "$path"
                        ;;
                    prepend)
                        echo "[test-worker] Prepending to file: $path" >&2
                        echo -e "$content\n$(cat "$path")" > "$path"
                        ;;
                    replace)
                        match=$(yq eval ".rules[$rule_idx].actions[$j].modify.match" "$fixture_file")
                        echo "[test-worker] Replacing in file: $path (pattern: $match)" >&2

                        # Create temp file for replacement
                        tmp_file=$(mktemp)
                        sed -E "s|$match|$content|g" "$path" > "$tmp_file"
                        mv "$tmp_file" "$path"
                        ;;
                    *)
                        echo "[test-worker] ERROR: Unknown modify operation: $operation" >&2
                        exit 1
                        ;;
                esac
                ;;

            delete)
                path=$(yq eval ".rules[$rule_idx].actions[$j].delete.path" "$fixture_file")

                echo "[test-worker] Deleting file: $path" >&2
                if [ -f "$path" ]; then
                    rm "$path"
                else
                    echo "[test-worker] WARNING: File to delete not found: $path" >&2
                fi
                ;;

            *)
                echo "[test-worker] ERROR: Unknown action type: $action_type" >&2
                exit 1
                ;;
        esac
    done
done

echo "[test-worker] Completed successfully (matched ${#matched_rules[@]} rule(s))" >&2

# Write exit.json for governator worker protocol
# This file is expected by the worker dispatcher to signal completion
if [ -n "${GOVERNATOR_WORKER_STATE_DIR:-}" ]; then
    exit_json="$GOVERNATOR_WORKER_STATE_DIR/exit.json"
    cat > "$exit_json" <<EOF
{
  "exit_code": 0,
  "finished_at": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "pid": $$
}
EOF
    echo "[test-worker] Wrote exit status to $exit_json" >&2
fi

exit 0
