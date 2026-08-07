package webui

import (
	"testing"
	"time"

	"github.com/agentic-demo/platform/internal/queue"
	"github.com/agentic-demo/platform/internal/reports"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapGenerationActivity_DBStatusFallback(t *testing.T) {
	now := time.Now()
	finished := now.Add(-time.Minute)
	jobs := []reports.GenerationJob{
		{
			ID: uuid.New(), TenantID: "t1", TaskID: "task-1",
			ReportType: "on_demand", Focus: "Growth",
			Status: reports.GenerationJobQueued, EnqueuedAt: now.Add(-2 * time.Minute),
		},
		{
			ID: uuid.New(), TenantID: "t1", TaskID: "task-2",
			ReportType: "weekly", Focus: "",
			Status: reports.GenerationJobSucceeded, EnqueuedAt: now.Add(-time.Hour), FinishedAt: &finished,
		},
		{
			ID: uuid.New(), TenantID: "t1", TaskID: "task-3",
			ReportType: "daily", Focus: "Signals",
			Status: reports.GenerationJobFailed, Error: "llm timeout", EnqueuedAt: now.Add(-2 * time.Hour),
		},
	}

	// No live queue states: DB statuses are shown as-is.
	data := MapGenerationActivity(jobs, nil)
	require.Len(t, data.Items, 3)

	assert.Equal(t, "Queued", data.Items[0].StatusLabel)
	assert.Equal(t, "info", data.Items[0].StatusIntent)
	assert.Equal(t, "On Demand", data.Items[0].TypeLabel)
	assert.Equal(t, "warning", data.Items[0].TypeIntent)

	assert.Equal(t, "Succeeded", data.Items[1].StatusLabel)
	assert.Equal(t, "success", data.Items[1].StatusIntent)

	assert.Equal(t, "Failed", data.Items[2].StatusLabel)
	assert.Equal(t, "error", data.Items[2].StatusIntent)
	assert.Equal(t, "llm timeout", data.Items[2].Error)
}

func TestMapGenerationActivity_LiveStateOverridesDB(t *testing.T) {
	now := time.Now()
	jobs := []reports.GenerationJob{
		{
			ID: uuid.New(), TenantID: "t1", TaskID: "task-1",
			ReportType: "on_demand",
			// Stale DB outcome — the queue says it is still running.
			Status: reports.GenerationJobFailed, Error: "stale error", EnqueuedAt: now,
		},
		{
			ID: uuid.New(), TenantID: "t1", TaskID: "task-2",
			ReportType: "monthly",
			Status:     reports.GenerationJobQueued, EnqueuedAt: now,
		},
	}
	states := map[string]queue.JobState{
		"task-1": {ID: "task-1", Status: queue.JobRunning},
		"task-2": {ID: "task-2", Status: queue.JobFailed, Error: "archived: boom"},
	}

	data := MapGenerationActivity(jobs, states)

	assert.Equal(t, "Running", data.Items[0].StatusLabel)
	assert.Equal(t, "", data.Items[0].Error, "stale DB error must not leak when live state is authoritative")
	assert.Equal(t, "Failed", data.Items[1].StatusLabel)
	assert.Equal(t, "archived: boom", data.Items[1].Error)
}

func TestMapGenerationActivity_UnknownStatus(t *testing.T) {
	jobs := []reports.GenerationJob{
		{
			ID: uuid.New(), TenantID: "t1", TaskID: "task-1",
			ReportType: "on_demand", Status: reports.GenerationJobStatus("bogus"), EnqueuedAt: time.Now(),
		},
	}

	data := MapGenerationActivity(jobs, nil)
	require.Len(t, data.Items, 1)
	assert.Equal(t, "Unknown", data.Items[0].StatusLabel)
	assert.Equal(t, "muted", data.Items[0].StatusIntent)
}

func TestMapGenerationActivity_Empty(t *testing.T) {
	data := MapGenerationActivity(nil, nil)
	assert.Empty(t, data.Items)
}
