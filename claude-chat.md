# Chat Mode for Architecture & Planning Phases

## Overview

This document specifies the design and implementation of an interactive "chat mode" for governator's architecture and planning phases. The goal is to allow the architect and planner agents to ask probing/clarifying questions before committing to designs and task decompositions.

**TODO Item:** `Chat mode during arch & planning to ask probing / clarifying questions.`

## Problem Statement

Currently, governator workers (including architect and planner) operate in "fire and forget" mode:
1. Worker receives prompt with all context (GOVERNATOR.md, role, task, etc.)
2. Worker executes autonomously
3. Worker produces artifacts and exits

This works well for implementation tasks with clear specifications, but architecture and planning phases often require clarification of ambiguous requirements, validation of assumptions, and discovery of unstated constraints. Without interactivity, the architect must either:
- Make assumptions that may be wrong
- Block the task entirely, requiring manual intervention

## Design Principles

The solution must align with governator's core philosophy:

1. **File-backed**: All conversations must be persisted to disk as markdown
2. **Git-tracked**: Discovery sessions become part of the auditable project history
3. **Deterministic**: The transcript provides reproducible context for downstream phases
4. **Explicit context**: No implicit memory or hidden state between sessions
5. **Role-bounded**: Chat mode only available for architect and planner roles

## Recommended Approach: Pre-flight Discovery Session

### Strategy

Add an optional but encouraged **discovery phase** before architecture bootstrap. This is a dedicated interactive session where the agent asks clarifying questions about the project intent. The conversation transcript becomes part of the architect's context.

```
                    ┌─────────────────┐
                    │  GOVERNATOR.md  │
                    │ (operator intent)│
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
     NEW PHASE ───▶ │    Discovery    │ ◀─── Interactive chat
                    │     Session     │      with operator
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │ discovery-      │
                    │ session.md      │ ◀─── Transcript artifact
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │  Architecture   │
                    │   Bootstrap     │ ◀─── Existing phase (now has
                    └─────────────────┘      discovery context)
```

### Why This Approach

1. **Clean separation**: Discovery is its own phase, not mixed into task execution
2. **Optional**: Projects with clear requirements can skip it
3. **Auditable**: Full transcript preserved in git
4. **Fits waterfall model**: Discovery → Architecture → Planning → Execution
5. **Simple implementation**: Leverages agent's native interactive mode

## Architecture

### New Files

```
_governator/
├── lib/
│   └── chat.sh                    # New library for chat functionality
├── templates/
│   └── discovery-prompt.md        # Template for discovery session
├── docs/
│   └── discovery-session.md       # Output: conversation transcript
```

### Configuration Additions

Add to `.governator/config.json` schema:

```json
{
  "discovery": {
    "required": false,
    "auto_prompt": true
  },
  "agents": {
    "providers": {
      "claude": {
        "bin": "claude",
        "args": ["--print"],
        "chat_args": []
      }
    }
  }
}
```

- `discovery.required`: If true, bootstrap cannot proceed without discovery-session.md
- `discovery.auto_prompt`: If true, prompt operator to run discovery when missing
- `chat_args`: Arguments for interactive mode (distinct from batch `args`)

### New Command

```bash
governator.sh chat [--role <role>]
```

- Default role: `architect` (for discovery)
- Also supports `planner` for planning-phase clarifications

## Implementation Design

### 1. Chat Library (`lib/chat.sh`)

```bash
#!/usr/bin/env bash

# Run an interactive chat session with the specified role's agent
# Arguments:
#   $1 - role (architect, planner)
#   $2 - session_type (discovery, clarification)
run_chat_session() {
    local role="${1:-architect}"
    local session_type="${2:-discovery}"
    local transcript_file="$DOCS_DIR/${session_type}-session.md"

    # Validate role is allowed for chat
    if [[ "$role" != "architect" && "$role" != "planner" ]]; then
        error_exit "Chat mode only available for architect and planner roles"
    fi

    # Check if session already exists
    if [[ -f "$transcript_file" ]]; then
        warn "Existing session found at $transcript_file"
        read -p "Overwrite? [y/N] " confirm
        [[ "$confirm" != "y" ]] && return 1
    fi

    # Build context and launch
    local context=$(build_chat_context "$role" "$session_type")
    launch_interactive_agent "$role" "$context" "$transcript_file"
}

# Assemble the initial context for the chat session
build_chat_context() {
    local role="$1"
    local session_type="$2"

    local context=""

    # Always include GOVERNATOR.md
    context+="# Project Intent"$'\n\n'
    context+="$(cat "$ROOT_DIR/GOVERNATOR.md")"$'\n\n'

    # Include discovery template
    if [[ -f "$TEMPLATES_DIR/${session_type}-prompt.md" ]]; then
        context+="---"$'\n\n'
        context+="$(cat "$TEMPLATES_DIR/${session_type}-prompt.md")"
    fi

    # For existing projects, include system discovery if present
    if [[ -f "$DOCS_DIR/existing-system-discovery.md" ]]; then
        context+=$'\n\n'"---"$'\n\n'
        context+="# Existing System Analysis"$'\n\n'
        context+="$(cat "$DOCS_DIR/existing-system-discovery.md")"
    fi

    echo "$context"
}

# Launch the agent in interactive mode
launch_interactive_agent() {
    local role="$1"
    local initial_context="$2"
    local transcript_file="$3"

    local provider=$(get_provider_for_role "$role")
    local bin=$(get_agent_bin "$provider")
    local chat_args=$(get_chat_args "$provider")

    # Write initial context to temp file
    local context_file=$(mktemp)
    echo "$initial_context" > "$context_file"

    # Initialize transcript
    {
        echo "# Discovery Session"
        echo ""
        echo "**Date:** $(date -Iseconds)"
        echo "**Role:** $role"
        echo "**Provider:** $provider"
        echo ""
        echo "---"
        echo ""
    } > "$transcript_file"

    echo "Starting interactive session with $provider..."
    echo "Transcript will be saved to: $transcript_file"
    echo ""
    echo "When finished, type '/done' or 'exit' to end the session."
    echo ""

    # Provider-specific launch
    case "$provider" in
        claude)
            launch_claude_chat "$bin" "$context_file" "$transcript_file"
            ;;
        gemini)
            launch_gemini_chat "$bin" "$context_file" "$transcript_file"
            ;;
        codex)
            launch_codex_chat "$bin" "$context_file" "$transcript_file"
            ;;
        *)
            error_exit "Chat mode not supported for provider: $provider"
            ;;
    esac

    rm -f "$context_file"

    echo ""
    echo "Session complete. Transcript saved to: $transcript_file"
}
```

### 2. Provider-Specific Chat Launchers

```bash
# Claude Code interactive session
launch_claude_chat() {
    local bin="$1"
    local context_file="$2"
    local transcript_file="$3"

    # Claude Code is interactive by default (without --print)
    # Pipe initial context, then user takes over

    # Option A: Simple - just launch with initial prompt
    # The user interacts directly, we capture what we can
    cat "$context_file" | $bin

    # After session, ask Claude to summarize
    echo ""
    read -p "Generate transcript summary? [Y/n] " gen_summary
    if [[ "$gen_summary" != "n" ]]; then
        $bin --print "Please provide a structured markdown summary of our discovery conversation. Include:
1. Key questions asked and answers received
2. Requirements clarified
3. Constraints identified
4. Assumptions validated or corrected
5. Open items still needing clarification" >> "$transcript_file"
    fi
}

# Gemini CLI interactive session
launch_gemini_chat() {
    local bin="$1"
    local context_file="$2"
    local transcript_file="$3"

    # Gemini CLI interactive mode
    cat "$context_file" | $bin

    # Similar summary generation
    # ...
}

# Codex interactive session
launch_codex_chat() {
    local bin="$1"
    local context_file="$2"
    local transcript_file="$3"

    # Codex without 'exec' is interactive
    cat "$context_file" | $bin

    # ...
}
```

### 3. Discovery Prompt Template

Create `_governator/templates/discovery-prompt.md`:

```markdown
# Discovery Session Instructions

You are conducting a requirements discovery session with the project operator. Your goal is to ask probing questions that will clarify the project intent and uncover information needed for sound architectural decisions.

## Your Objectives

1. **Clarify ambiguities** in the project description
2. **Uncover unstated requirements** (security, scale, compliance, etc.)
3. **Validate assumptions** you might otherwise make
4. **Identify constraints** (timeline, budget, team skills, existing systems)
5. **Understand priorities** (what's essential vs. nice-to-have)
6. **Discover integration points** with external systems

## How to Conduct This Session

1. Start by acknowledging you've read the project intent
2. Ask 2-3 focused questions at a time (not overwhelming lists)
3. Listen to answers and ask follow-up questions
4. Probe for specifics when answers are vague
5. Summarize your understanding periodically
6. When you have sufficient clarity, say "DISCOVERY COMPLETE"

## Question Categories to Cover

- **Users**: Who are the primary users? What are their technical capabilities?
- **Scale**: Expected load, data volumes, growth projections?
- **Security**: Authentication needs? Sensitive data? Compliance requirements?
- **Integration**: External APIs, databases, services to connect with?
- **Constraints**: Technology preferences/restrictions? Team expertise?
- **Quality**: Performance requirements? Availability needs?
- **Timeline**: Hard deadlines? Phased delivery acceptable?

## Important

- Do NOT start designing or proposing solutions
- Do NOT make assumptions - ask instead
- Focus on understanding, not solutioning
- Keep questions conversational, not interrogative

Begin by introducing yourself and asking your first questions.
```

### 4. Bootstrap Integration

Modify `lib/bootstrap.sh` to check for discovery session:

```bash
# Add to bootstrap_gate_allows_assignment() or similar

check_discovery_session() {
    local discovery_file="$DOCS_DIR/discovery-session.md"
    local config_required=$(read_config ".discovery.required")
    local config_auto_prompt=$(read_config ".discovery.auto_prompt")

    if [[ ! -f "$discovery_file" ]]; then
        if [[ "$config_required" == "true" ]]; then
            error_exit "Discovery session required. Run: governator.sh chat"
        elif [[ "$config_auto_prompt" == "true" ]]; then
            echo ""
            echo "No discovery session found."
            read -p "Run discovery session before architecture bootstrap? [Y/n] " run_discovery
            if [[ "$run_discovery" != "n" ]]; then
                run_chat_session "architect" "discovery"
            fi
        fi
    fi
}
```

### 5. Include Discovery in Architect Context

Modify `lib/workers.sh` to include discovery transcript when spawning architect:

```bash
build_worker_prompt() {
    local role="$1"
    local task_file="$2"

    # ... existing prompt building ...

    # Add discovery session if present and role is architect
    if [[ "$role" == "architect" && -f "$DOCS_DIR/discovery-session.md" ]]; then
        prompt+=$'\n\n'"# Discovery Session Transcript"$'\n\n'
        prompt+="$(cat "$DOCS_DIR/discovery-session.md")"
    fi

    # ... rest of prompt ...
}
```

### 6. Main Command Entry Point

Add to `governator.sh`:

```bash
chat_cmd() {
    local role="architect"
    local session_type="discovery"

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --role)
                role="$2"
                shift 2
                ;;
            --planning)
                role="planner"
                session_type="planning-clarification"
                shift
                ;;
            *)
                shift
                ;;
        esac
    done

    require_governator_doc
    source "$LIB_DIR/chat.sh"
    run_chat_session "$role" "$session_type"
}

# In main case statement:
case "$1" in
    # ... existing commands ...
    chat)
        shift
        chat_cmd "$@"
        ;;
esac
```

## Transcript Format

The generated `discovery-session.md` should follow this structure:

```markdown
# Discovery Session

**Date:** 2024-01-15T10:30:00-05:00
**Role:** architect
**Provider:** claude

---

## Session Summary

[Agent-generated summary of key findings]

### Requirements Clarified
- [Bullet points]

### Constraints Identified
- [Bullet points]

### Assumptions Validated
- [Bullet points]

### Open Items
- [Any items still needing clarification]

---

## Full Transcript

[If captured - raw conversation log]
```

## Future Enhancements (Out of Scope for v1)

1. **In-flight clarification**: Allow workers to pause and request clarification mid-task via a `pending-questions.md` protocol
2. **Session resume**: Ability to continue a previous discovery session
3. **Multi-provider transcript export**: Standardized transcript extraction across all agent providers
4. **Structured Q&A format**: Machine-readable question/answer pairs for better downstream processing

## Testing Strategy

1. **Unit tests**: Mock agent binaries to verify prompt construction and file handling
2. **Integration test**: Full discovery flow with a test GOVERNATOR.md
3. **Provider tests**: Verify chat launch works for each supported provider

## Implementation Checklist

- [ ] Create `lib/chat.sh` with core chat functionality
- [ ] Create `templates/discovery-prompt.md`
- [ ] Add `chat` command to `governator.sh`
- [ ] Add `chat_args` to provider config schema
- [ ] Modify `bootstrap.sh` to check for discovery session
- [ ] Modify `workers.sh` to include discovery transcript in architect context
- [ ] Update `README.md` with chat command documentation
- [ ] Add discovery config options to `config.json` schema
- [ ] Test with Claude Code provider
- [ ] Test with Gemini CLI provider
- [ ] Test with Codex provider
