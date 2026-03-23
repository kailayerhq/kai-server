package api

import (
	"encoding/json"
	"log"
	"time"

	"kailab-control/internal/cron"
	"kailab-control/internal/model"
	"kailab-control/internal/workflow"
)

// StartScheduler starts a background goroutine that checks for and triggers
// scheduled workflows every minute, and reaps stale jobs every 2 minutes.
func (h *Handler) StartScheduler(done <-chan struct{}) {
	go func() {
		// Align to the start of the next minute for predictable scheduling.
		now := time.Now()
		next := now.Truncate(time.Minute).Add(time.Minute)
		time.Sleep(next.Sub(now))

		scheduleTicker := time.NewTicker(1 * time.Minute)
		defer scheduleTicker.Stop()

		reapTicker := time.NewTicker(2 * time.Minute)
		defer reapTicker.Stop()

		// Run immediately at the aligned minute, then on each tick.
		h.runScheduledWorkflows()
		h.reapStaleJobs()

		for {
			select {
			case <-scheduleTicker.C:
				h.runScheduledWorkflows()
			case <-reapTicker.C:
				h.reapStaleJobs()
			case <-done:
				return
			}
		}
	}()
}

// reapStaleJobs finds jobs that have missed heartbeats (5 min) or exceeded
// the fallback timeout (45 min for jobs without heartbeats), marks them
// as failed, and completes the parent run.
func (h *Handler) reapStaleJobs() {
	affectedRunIDs, err := h.db.FailStaleJobs(5*time.Minute, 45*time.Minute)
	if err != nil {
		log.Printf("reaper: failed to reap stale jobs: %v", err)
		return
	}

	for _, runID := range affectedRunIDs {
		log.Printf("reaper: failed stale jobs in run %s", runID)
		h.checkAndCompleteWorkflowRun(runID)
	}
}

// runScheduledWorkflows checks all schedule-triggered workflows and creates runs
// for any whose cron expressions match the current minute.
func (h *Handler) runScheduledWorkflows() {
	now := time.Now().UTC()

	workflows, err := h.db.ListAllScheduleWorkflows()
	if err != nil {
		log.Printf("scheduler: failed to list schedule workflows: %v", err)
		return
	}

	if len(workflows) == 0 {
		return
	}

	for _, wf := range workflows {
		h.checkAndTriggerSchedule(wf, now)
	}
}

// checkAndTriggerSchedule checks if a workflow's schedule matches the current time
// and creates a run if so.
func (h *Handler) checkAndTriggerSchedule(wf *model.Workflow, now time.Time) {
	parsed, err := workflow.FromJSON(wf.ParsedJSON)
	if err != nil {
		log.Printf("scheduler: failed to parse workflow %s: %v", wf.ID, err)
		return
	}

	if len(parsed.On.Schedule) == 0 {
		return
	}

	// Check each cron expression
	for _, sched := range parsed.On.Schedule {
		cronSched, err := cron.Parse(sched.Cron)
		if err != nil {
			log.Printf("scheduler: invalid cron expression %q in workflow %s: %v", sched.Cron, wf.ID, err)
			continue
		}

		if !cronSched.Match(now) {
			continue
		}

		// This schedule matches — create a run.
		log.Printf("scheduler: triggering workflow %s (%s) on cron %q", wf.Name, wf.ID, sched.Cron)

		// Get repo and org info for the ref.
		repo, err := h.db.GetRepoByID(wf.RepoID)
		if err != nil {
			log.Printf("scheduler: failed to get repo %s: %v", wf.RepoID, err)
			return
		}
		org, err := h.db.GetOrgByID(repo.OrgID)
		if err != nil {
			log.Printf("scheduler: failed to get org %s: %v", repo.OrgID, err)
			return
		}

		// Schedule triggers use the default branch.
		ref := "refs/heads/main"

		payload := map[string]interface{}{
			"schedule": sched.Cron,
		}
		payloadJSON, _ := json.Marshal(payload)

		run, err := h.db.CreateWorkflowRun(wf.ID, repo.ID, model.TriggerSchedule, ref, "", string(payloadJSON), "")
		if err != nil {
			log.Printf("scheduler: failed to create run for workflow %s: %v", wf.ID, err)
			return
		}

		if err := h.createJobsFromWorkflow(wf, run); err != nil {
			log.Printf("scheduler: failed to create jobs for run %s: %v", run.ID, err)
			return
		}

		log.Printf("scheduler: created run %s for %s/%s workflow %s", run.ID, org.Slug, repo.Name, wf.Name)

		// Only trigger once per workflow per tick (first matching cron wins).
		return
	}
}
