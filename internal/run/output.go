// Package run defines helpers for emitting standardized run output events.
package run

import (
	"io"
	"strconv"
	"strings"
)

const (
	mergeStageName      = "merge"
	defaultUnknownToken = "unknown"
	planRequiredCommand = "governator plan"
)

const (
	eventStatusStart    = "start"
	eventStatusComplete = "complete"
	eventStatusFailure  = "failure"
	eventStatusTimeout  = "timeout"
)

type taskEventAttr struct {
	key   string
	value string
	quote bool
}

// emitTaskStart reports that a worker stage is starting for a task.
func emitTaskStart(out io.Writer, taskID string, role string, stage string) {
	emitTaskStatus(out, taskID, role, stage, eventStatusStart, "", nil)
}

// emitTaskComplete reports that a worker stage completed successfully for a task.
func emitTaskComplete(out io.Writer, taskID string, role string, stage string) {
	emitTaskStatus(out, taskID, role, stage, eventStatusComplete, "", nil)
}

// emitTaskFailure reports that a worker stage failed for a task.
func emitTaskFailure(out io.Writer, taskID string, role string, stage string, reason string) {
	if strings.TrimSpace(reason) == "" {
		reason = "unknown failure"
	}
	emitTaskStatus(out, taskID, role, stage, eventStatusFailure, reason, nil)
}

// emitTaskTimeout reports that a worker stage timed out for a task.
func emitTaskTimeout(out io.Writer, taskID string, role string, stage string, reason string, timeoutSeconds int) {
	if strings.TrimSpace(reason) == "" {
		reason = "timeout"
	}
	attrs := []taskEventAttr{
		{key: "timeout_seconds", value: strconv.Itoa(timeoutSeconds)},
	}
	emitTaskStatus(out, taskID, role, stage, eventStatusTimeout, reason, attrs)
}

// emitPlanningDriftMessage reports that planning drift was detected.
func emitPlanningDriftMessage(out io.Writer, detail string) {
	if out == nil {
		return
	}
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = "planning drift detected"
	}
	emitPlanningMessage(out, detail)
}

func emitPlanningMessage(out io.Writer, detail string) {
	if out == nil {
		return
	}
	_, _ = out.Write([]byte(
		"planning=drift status=blocked reason=" +
			strconv.Quote(detail) +
			" next_step=" +
			strconv.Quote(planRequiredCommand) +
			"\n",
	))
}

func emitTaskStatus(out io.Writer, taskID string, role string, stage string, status string, reason string, attrs []taskEventAttr) {
	if out == nil {
		return
	}
	cleanTaskID := normalizeToken(taskID)
	cleanRole := normalizeToken(role)
	cleanStage := normalizeToken(stage)

	builder := &strings.Builder{}
	builder.WriteString("task=")
	builder.WriteString(cleanTaskID)
	builder.WriteString(" role=")
	builder.WriteString(cleanRole)
	builder.WriteString(" stage=")
	builder.WriteString(cleanStage)
	builder.WriteString(" status=")
	builder.WriteString(status)

	if strings.TrimSpace(reason) != "" {
		builder.WriteString(" reason=")
		builder.WriteString(strconv.Quote(reason))
	}

	for _, attr := range attrs {
		builder.WriteString(" ")
		builder.WriteString(attr.key)
		builder.WriteString("=")
		if attr.quote {
			builder.WriteString(strconv.Quote(attr.value))
		} else {
			builder.WriteString(attr.value)
		}
	}

	builder.WriteByte('\n')
	_, _ = out.Write([]byte(builder.String()))
}

func normalizeToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultUnknownToken
	}
	return value
}
