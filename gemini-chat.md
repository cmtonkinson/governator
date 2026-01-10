# Implementation Plan: Governator Chat Mode

This document outlines the recommended strategy, architecture, and design for implementing an interactive "chat mode" feature in the `governator` tool. The goal is to allow a user to have a conversation with an AI agent to clarify and refine the project plan after the initial architecture and high-level planning are complete, but before detailed task breakdown.

## 1. User Workflow

1.  **Initiate and Pause:** The user starts the process with a new `--chat` flag:
    ```bash
    governator.sh run --chat
    ```
2.  **Initial Planning:** `governator` runs as usual, generating the architecture documents, milestones, and epics.
3.  **Pause for Chat:** After this high-level planning, the system pauses and creates a lock file. It informs the user that it's waiting for a chat session.
4.  **Start Chat Session:** The user initiates the interactive chat session with a new `chat` command:
    ```bash
    governator.sh chat
    ```
5.  **Interactive Refinement:** The user has a conversation with the AI agent to ask questions, resolve ambiguities, and refine the plan. The agent's responses are guided by the existing architectural context.
6.  **Resume Planning:** When the user ends the chat session (e.g., by typing `exit`), the lock file is removed. The user can then resume the planning process by running the `run` command again:
    ```bash
    governator.sh run
    ```
7.  **Final Plan:** `governator` proceeds to generate the detailed tasks, now informed by the clarifications from the chat session.

## 2. Architecture and Design

The implementation will be contained within the existing `_governator` structure, primarily through new and modified shell scripts.

### 2.1. New Files

*   `_governator/lib/chat.sh`: A new library script to encapsulate all logic for the chat session.
*   `_governator/custom-prompts/chat.md`: A new prompt file to define the AI agent's persona and instructions for the chat.
*   `_governator/reasoning/chat_session_<timestamp>.md`: A log file to store the transcript of each chat session for auditing and review.

### 2.2. Modified Files

*   `_governator/governator.sh`:
    *   Add parsing for the `--chat` flag in the `run` command.
    *   Add a new `chat` command to the main command dispatcher.
    *   Source the new `_governator/lib/chat.sh` script.
*   `_governator/lib/bootstrap.sh` (or similar):
    *   At the end of the high-level planning phase, check for the `--chat` flag and, if present, create the pause lock file.
*   `_governator/lib/locks.sh`:
    *   Add functions to create, check for, and remove the new pause lock file (e.g., `.governator/state/paused_for_chat`).
*   `_governator/lib/core.sh`:
    *   The `run` command logic will be modified to check for the pause lock file at the beginning of its execution.

## 3. Core Implementation Details

### 3.1. The Chat Loop (`chat.sh`)

The `chat.sh` script will implement a read-eval-print loop (REPL) that:
1.  Presents a `You: ` prompt to the user.
2.  Reads user input.
3.  Manages the conversation history (see below).
4.  Calls the configured AI agent.
5.  Prints the agent's response to the console.
6.  Repeats until the user types `exit` or `quit`.

### 3.2. Context Management and Agent Interaction

The chat will be powered by the existing agent provider mechanism in `governator`.

*   **Client-Side Context:** The chat will be stateless from the agent's perspective. The `chat.sh` script will be responsible for maintaining context by storing the full conversation history in a local file.
*   **Continuous Re-invocation:** With each turn of the conversation, the `chat.sh` script will "re-invoke the agent continually" by sending the *entire* conversation history, along with the system prompt from `chat.md` and the new user input, to the agent's API.
*   **Token Limit Management:** To prevent exceeding the agent's token limit on long conversations, a strategy of truncating the oldest parts of the conversation history from the prompt should be implemented.

### 3.3. The `call_agent` function

The `chat.sh` script will contain a `call_agent` function that:
1.  Determines the configured agent provider (e.g., from an environment variable).
2.  Reads the conversation history file.
3.  Constructs the full prompt for the API call.
4.  Uses `curl` to send the request to the agent's API.
5.  Parses the JSON response to extract the agent's message.

This design ensures that the chat feature is modular, leverages the existing agent abstraction layer, and is robust enough to handle a real-world conversational workflow.
