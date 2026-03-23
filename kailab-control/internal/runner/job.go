package runner

import (
	"context"
	"io"
	"time"
)

// Job is the interface that both Kubernetes pods and local processes implement
// for executing CI steps.
type Job interface {
	// ExecuteStep runs a single workflow step (either a "run:" command or "uses:" action).
	ExecuteStep(ctx context.Context, stepDef *StepDefinition, jobContext map[string]interface{}, logWriter io.Writer) (*ExecutionResult, error)

	// ExecuteCommandWithTimeout runs a helper command with a maximum duration.
	// Used for internal operations like capturing GITHUB_OUTPUT.
	ExecuteCommandWithTimeout(ctx context.Context, timeout time.Duration, script, shell string, logWriter io.Writer) (*ExecutionResult, error)

	// Cleanup releases resources (deletes pod, removes workspace, etc.)
	Cleanup(ctx context.Context) error
}

// JobCreator creates Jobs for executing CI workflow jobs.
type JobCreator interface {
	// CreateJob sets up an execution environment for a job (pod, workspace dir, etc.)
	CreateJob(ctx context.Context, jobID, jobName string, jobContext map[string]interface{}) (Job, error)

	// GCStalePods cleans up stale resources (pods, old workspaces, etc.)
	GCStalePods(ctx context.Context)
}
