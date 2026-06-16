package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/z46-dev/golog"
	"github.com/z46-dev/overlord-ipa/db"
)

type JobRepository interface {
	ListJobs(ctx context.Context) ([]db.Job, error)
	GetJob(ctx context.Context, id int) (*db.Job, error)
	GetJobRun(ctx context.Context, id int) (*db.JobRun, error)
	GetJobByName(ctx context.Context, name string) (*db.Job, error)
	InsertJob(ctx context.Context, job *db.Job) error
	UpdateJob(ctx context.Context, job *db.Job) error
	InsertJobRun(ctx context.Context, run *db.JobRun) error
	UpdateJobRun(ctx context.Context, run *db.JobRun) error
	InsertJobAction(ctx context.Context, action *db.JobAction) error
	UpdateJobAction(ctx context.Context, action *db.JobAction) error
	InsertJobActionRun(ctx context.Context, run *db.JobActionRun) error
	UpdateJobActionRun(ctx context.Context, run *db.JobActionRun) error
	ListRecentJobRuns(ctx context.Context, limit int) ([]db.JobRun, error)
	ListJobActionRunsByRunIDs(ctx context.Context, runIDs []int) ([]db.JobActionRun, error)
	ListJobHostResultsByRunIDs(ctx context.Context, runIDs []int) ([]db.JobHostResult, error)
	UpsertHost(ctx context.Context, host *db.Host) error
	InsertJobHostResult(ctx context.Context, result *db.JobHostResult) error
	DeleteJobActions(ctx context.Context, jobID int) error
	DeleteJobAction(ctx context.Context, actionID int) error
	ListJobActions(ctx context.Context, jobID int) ([]db.JobAction, error)
}

type JobHostProvider interface {
	GetHostGroups(ctx context.Context) ([]string, error)
	GetHostsForGroups(ctx context.Context, groups []string) ([]db.Host, error)
}

type ActionFileValidator interface {
	ValidateActionFile(ctx context.Context, path string, actionType db.JobActionType) error
}

type Action interface {
	Execute(ctx context.Context, run *db.JobRun) error
}

type ActionFactory interface {
	NewAction(action db.JobAction) (Action, error)
}

type JobService struct {
	repository JobRepository
	actions    ActionFactory
	groups     JobHostProvider
	files      ActionFileValidator
	queue      *JobQueue
	ansible    *GoAnsibleRunner
	inventory  *AnsibleInventoryWriter
	log        *golog.Logger
}

type JobActionInput struct {
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	Type            db.JobActionType `json:"type"`
	FilePath        string           `json:"file_path"`
	Arguments       []string         `json:"arguments"`
	ContinueOnError bool             `json:"continue_on_error"`
	TimeoutSeconds  int64            `json:"timeout_seconds"`
}

type JobInput struct {
	Name             string              `json:"name"`
	Description      string              `json:"description"`
	Enabled          bool                `json:"enabled"`
	IntervalSeconds  int64               `json:"interval_seconds"`
	ScheduleType     db.ScheduleType     `json:"schedule_type"`
	CronExpr         string              `json:"cron_expr"`
	LongevityType    db.JobLongevityType `json:"longevity_type"`
	MaxRuns          int                 `json:"max_runs"`
	DisableAfter     time.Time           `json:"disable_after"`
	TargetHostgroups []string            `json:"target_hostgroups"`
	Actions          []JobActionInput    `json:"actions"`
}

// NewJobService creates the job business service.
func NewJobService(repository JobRepository, actions ActionFactory, groups JobHostProvider, files ActionFileValidator, queue *JobQueue, ansible *GoAnsibleRunner, inventory *AnsibleInventoryWriter, logger *golog.Logger) (service *JobService) {
	service = &JobService{
		repository: repository,
		actions:    actions,
		groups:     groups,
		files:      files,
		queue:      queue,
		ansible:    ansible,
		inventory:  inventory,
		log:        serviceLogger(logger, "[JOBS]", golog.BoldBlue),
	}
	return
}

// EnsureDefaultJobs creates protected built-in job examples if missing.
func (s *JobService) EnsureDefaultJobs(ctx context.Context) (err error) {
	var (
		defaults []JobInput = defaultJobInputs()
		existing *db.Job
	)

	for _, input := range defaults {
		if existing, err = s.repository.GetJobByName(ctx, input.Name); err != nil {
			err = NewPersistenceError("get default job", err)
			return
		}

		if existing != nil && !existing.Protected {
			continue
		}

		if existing != nil {
			if err = s.updateProtectedDefaultJob(ctx, existing, input); err != nil {
				return
			}

			continue
		}

		if _, err = s.createJob(ctx, input, true); err != nil {
			return
		}
	}

	return
}

// updateProtectedDefaultJob refreshes built-in metadata while preserving operator-controlled settings.
func (s *JobService) updateProtectedDefaultJob(ctx context.Context, job *db.Job, input JobInput) (err error) {
	input = normalizeJobInput(input)
	job.Description = strings.TrimSpace(input.Description)
	job.IntervalSeconds = 0
	job.ScheduleType = input.ScheduleType
	job.CronExpr = strings.TrimSpace(input.CronExpr)
	job.UpdatedAt = time.Now().UTC()

	if job.LongevityType == db.JobLongevityTypeUnknown {
		job.LongevityType = normalizeLongevityType(input.LongevityType)
	}

	if err = s.repository.UpdateJob(ctx, job); err != nil {
		err = NewPersistenceError("update default job", err)
		return
	}

	if err = s.replaceJobActions(ctx, job.ID, input.Actions); err != nil {
		return
	}

	return
}

// ListJobs returns all configured jobs.
func (s *JobService) ListJobs(ctx context.Context) (jobs []db.Job, err error) {
	if jobs, err = s.repository.ListJobs(ctx); err != nil {
		err = NewPersistenceError("list jobs", err)
		return
	}

	return
}

// ListJobActions returns persisted actions for a job.
func (s *JobService) ListJobActions(ctx context.Context, jobID int) (actions []db.JobAction, err error) {
	if actions, err = s.repository.ListJobActions(ctx, jobID); err != nil {
		err = NewPersistenceError("list job actions", err)
		return
	}

	return
}

// ListRecentJobRuns returns recent execution records.
func (s *JobService) ListRecentJobRuns(ctx context.Context, limit int) (runs []db.JobRun, err error) {
	if runs, err = s.repository.ListRecentJobRuns(ctx, limit); err != nil {
		err = NewPersistenceError("list recent job runs", err)
		return
	}

	return
}

// ListJobActionRunsByRunIDs returns action-level output for recent runs.
func (s *JobService) ListJobActionRunsByRunIDs(ctx context.Context, runIDs []int) (runs []db.JobActionRun, err error) {
	if runs, err = s.repository.ListJobActionRunsByRunIDs(ctx, runIDs); err != nil {
		err = NewPersistenceError("list job action runs", err)
		return
	}

	return
}

// ListJobHostResultsByRunIDs returns per-host output for recent runs.
func (s *JobService) ListJobHostResultsByRunIDs(ctx context.Context, runIDs []int) (results []db.JobHostResult, err error) {
	if results, err = s.repository.ListJobHostResultsByRunIDs(ctx, runIDs); err != nil {
		err = NewPersistenceError("list job host results", err)
		return
	}

	return
}

// CreateJob validates and persists a new custom job.
func (s *JobService) CreateJob(ctx context.Context, input JobInput) (job *db.Job, err error) {
	input = normalizeJobInput(input)
	job, err = s.createJob(ctx, input, false)
	return
}

// UpdateJob validates and persists an existing job.
func (s *JobService) UpdateJob(ctx context.Context, jobID int, input JobInput) (job *db.Job, err error) {
	var existing *db.Job
	input = normalizeJobInput(input)

	if existing, err = s.repository.GetJob(ctx, jobID); err != nil {
		err = NewPersistenceError("get job", err)
		return
	}

	if existing == nil {
		err = NewNotFoundError("job not found", nil)
		return
	}

	if err = s.validateJobFields(ctx, input); err != nil {
		return
	}

	if !existing.Protected {
		if len(input.Actions) == 0 {
			err = NewInvalidInputError("at least one action is required", nil)
			return
		}

		if err = s.validateActions(ctx, input.Actions); err != nil {
			return
		}
	}

	existing.Description = strings.TrimSpace(input.Description)
	existing.Enabled = input.Enabled
	existing.IntervalSeconds = 0
	existing.ScheduleType = input.ScheduleType
	existing.CronExpr = strings.TrimSpace(input.CronExpr)
	existing.LongevityType = normalizeLongevityType(input.LongevityType)
	existing.MaxRuns = input.MaxRuns
	existing.DisableAfter = normalizeDisableAfter(input)
	existing.TargetHostgroups = normalizeStringList(input.TargetHostgroups)
	existing.UpdatedAt = time.Now().UTC()

	if !existing.Protected {
		existing.Name = strings.TrimSpace(input.Name)
	}

	if err = s.repository.UpdateJob(ctx, existing); err != nil {
		err = NewPersistenceError("update job", err)
		return
	}

	if !existing.Protected {
		if err = s.replaceJobActions(ctx, existing.ID, input.Actions); err != nil {
			return
		}
	}

	job = existing
	return
}

// RunJob queues a manually-triggered job for worker execution.
func (s *JobService) RunJob(ctx context.Context, jobID int, triggeredBy string) (run *db.JobRun, err error) {
	run, err = s.runJob(ctx, jobID, triggeredBy, db.JobRunTriggerTypeManual)
	return
}

// RunScheduledJob queues a scheduler-triggered job for worker execution.
func (s *JobService) RunScheduledJob(ctx context.Context, jobID int) (run *db.JobRun, err error) {
	run, err = s.runJob(ctx, jobID, "scheduler", db.JobRunTriggerTypeSchedule)
	return
}

func (s *JobService) runJob(ctx context.Context, jobID int, triggeredBy string, triggerType db.JobRunTriggerType) (run *db.JobRun, err error) {
	var (
		job     *db.Job
		actions []db.JobAction
		taskID  int
	)

	if s.queue == nil {
		err = NewExecutionError("job queue is not configured", nil)
		return
	}

	if job, err = s.repository.GetJob(ctx, jobID); err != nil {
		err = NewPersistenceError("get job", err)
		return
	}

	if job == nil {
		err = NewNotFoundError("job not found", nil)
		return
	}

	if len(job.TargetHostgroups) == 0 {
		err = NewInvalidInputError("job requires at least one target host group before it can run", nil)
		return
	}

	if err = s.validateTargetGroups(ctx, job.TargetHostgroups); err != nil {
		return
	}

	if actions, err = s.repository.ListJobActions(ctx, job.ID); err != nil {
		err = NewPersistenceError("list job actions", err)
		return
	}

	if len(actions) == 0 {
		err = NewInvalidInputError("job has no actions", nil)
		return
	}

	if s.log != nil {
		s.log.Infof("Queueing job job_id=%d name=%s triggered_by=%s groups=%v actions=%d\n", job.ID, job.Name, triggeredBy, job.TargetHostgroups, len(actions))
	}

	run = &db.JobRun{
		JobID:            uint64(job.ID),
		Status:           db.JobRunStatusQueued,
		TriggerType:      triggerType,
		TriggeredBy:      triggeredBy,
		StartTime:        time.Now().UTC(),
		TargetHostgroups: job.TargetHostgroups,
	}

	if err = s.repository.InsertJobRun(ctx, run); err != nil {
		err = NewPersistenceError("insert job run", err)
		return
	}

	if taskID, err = s.queue.EnqueueJobRun(ctx, JobRunTaskPayload{
		JobID:       job.ID,
		JobRunID:    run.ID,
		TriggeredBy: triggeredBy,
		HostGroups:  job.TargetHostgroups,
	}); err != nil {
		var updateErr error
		run.Status = db.JobRunStatusFailed
		run.Error = err.Error()
		run.EndTime = time.Now().UTC()
		if updateErr = s.repository.UpdateJobRun(ctx, run); updateErr != nil {
			err = NewPersistenceError("update failed job run", updateErr)
			return
		}

		return
	}

	run.Summary = fmt.Sprintf("queued as task %d", taskID)
	if err = s.repository.UpdateJobRun(ctx, run); err != nil {
		err = NewPersistenceError("update queued job run", err)
		return
	}

	if s.log != nil {
		s.log.Infof("Queued job run job_id=%d run_id=%d task_id=%d\n", job.ID, run.ID, taskID)
	}

	return
}

// ExecuteQueuedJob executes a queued job run from the Gasket worker.
func (s *JobService) ExecuteQueuedJob(ctx context.Context, payload JobRunTaskPayload) (data []byte, err error) {
	var (
		job       *db.Job
		run       *db.JobRun
		actions   []db.JobAction
		hosts     []db.Host
		inventory AnsibleInventory
		groups    []string
	)

	if s.ansible == nil || s.inventory == nil {
		err = NewExecutionError("job execution dependencies are not configured", nil)
		return
	}

	if s.log != nil {
		s.log.Infof("Worker starting job run job_id=%d run_id=%d triggered_by=%s\n", payload.JobID, payload.JobRunID, payload.TriggeredBy)
	}

	if run, err = s.repository.GetJobRun(ctx, payload.JobRunID); err != nil {
		err = NewPersistenceError("get job run", err)
		return
	}

	if run == nil {
		err = NewNotFoundError("job run not found", nil)
		return
	}

	if isTerminalJobRunStatus(run.Status) {
		data = []byte(run.Summary)
		if len(data) == 0 {
			data = []byte(run.Error)
		}

		if s.log != nil {
			s.log.Infof("Skipping already completed job run job_id=%d run_id=%d status=%d\n", payload.JobID, run.ID, run.Status)
		}

		return
	}

	if job, err = s.repository.GetJob(ctx, payload.JobID); err != nil {
		err = NewPersistenceError("get queued job", err)
		return
	}

	if job == nil {
		err = NewNotFoundError("queued job not found", nil)
		return
	}

	if actions, err = s.repository.ListJobActions(ctx, job.ID); err != nil {
		err = NewPersistenceError("list queued job actions", err)
		return
	}

	run.Status = db.JobRunStatusRunning
	run.StartTime = time.Now().UTC()
	if err = s.repository.UpdateJobRun(ctx, run); err != nil {
		err = NewPersistenceError("mark job run running", err)
		return
	}

	groups = normalizeStringList(run.TargetHostgroups)
	if len(groups) == 0 {
		groups = normalizeStringList(payload.HostGroups)
	}

	if len(groups) == 0 {
		groups = normalizeStringList(job.TargetHostgroups)
	}

	run.TargetHostgroups = groups
	if s.log != nil {
		s.log.Infof("Resolving job run targets job_id=%d run_id=%d groups=%v\n", job.ID, run.ID, groups)
	}

	if hosts, err = s.groups.GetHostsForGroups(ctx, groups); err != nil {
		err = s.failRun(ctx, run, fmt.Errorf("resolve target hosts: %w", err))
		return
	}

	if s.log != nil {
		s.log.Infof("Resolved job run targets job_id=%d run_id=%d hosts=%d\n", job.ID, run.ID, len(hosts))
	}

	if inventory, err = s.inventory.WriteJobInventory(ctx, run.ID, hosts); err != nil {
		err = s.failRun(ctx, run, err)
		return
	}

	run.TargetHosts = inventory.Hostnames
	run.TotalHosts = len(inventory.Hostnames)
	if err = s.repository.UpdateJobRun(ctx, run); err != nil {
		err = NewPersistenceError("update job run targets", err)
		return
	}

	if err = s.executeActions(ctx, run, actions, inventory); err != nil {
		return
	}

	data = []byte(run.Summary)
	if s.log != nil {
		s.log.Infof("Worker finished job run job_id=%d run_id=%d status=%d summary=%s\n", job.ID, run.ID, run.Status, run.Summary)
	}

	return
}

// executeActions executes each action and updates run status.
func (s *JobService) executeActions(ctx context.Context, run *db.JobRun, actions []db.JobAction, inventory AnsibleInventory) (err error) {
	var (
		actionRun db.JobActionRun
		output    AnsibleRunOutput
		actionErr error
	)

	for _, action := range actions {
		if s.log != nil {
			s.log.Infof("Starting job action run_id=%d action_id=%d name=%s type=%d file=%s\n", run.ID, action.ID, action.Name, action.Type, action.FilePath)
		}

		actionRun = db.JobActionRun{
			JobRunID:    uint64(run.ID),
			JobActionID: uint64(action.ID),
			Status:      db.JobRunStatusRunning,
			StartTime:   time.Now().UTC(),
		}

		if err = s.repository.InsertJobActionRun(ctx, &actionRun); err != nil {
			err = NewPersistenceError("insert job action run", err)
			return
		}

		output, actionErr = s.executeAction(ctx, action, inventory)
		actionRun.Stdout = output.Stdout
		actionRun.Stderr = output.Stderr
		actionRun.EndTime = time.Now().UTC()
		if actionErr != nil {
			actionRun.Status = db.JobRunStatusFailed
			actionRun.Error = actionErr.Error()
			if s.log != nil {
				s.log.Errorf("Job action failed run_id=%d action_id=%d error=%v\n", run.ID, action.ID, actionErr)
			}
		} else {
			actionRun.Status = db.JobRunStatusSuccess
			if s.log != nil {
				s.log.Infof("Job action completed run_id=%d action_id=%d stdout_bytes=%d stderr_bytes=%d\n", run.ID, action.ID, len(output.Stdout), len(output.Stderr))
			}
		}

		if err = s.persistActionHostResults(ctx, run, action, output, inventory); err != nil {
			return
		}

		if err = s.repository.UpdateJobActionRun(ctx, &actionRun); err != nil {
			err = NewPersistenceError("update job action run", err)
			return
		}

		if actionErr != nil && !action.ContinueOnError {
			err = s.failRun(ctx, run, actionErr)
			return
		}
	}

	run.Status = db.JobRunStatusSuccess
	run.EndTime = time.Now().UTC()
	run.SuccessHosts = run.TotalHosts
	run.FailedHosts = 0
	run.SkippedHosts = 0
	run.Summary = fmt.Sprintf("completed %d action(s) across %d host(s)", len(actions), run.TotalHosts)
	if err = s.repository.UpdateJobRun(ctx, run); err != nil {
		err = NewPersistenceError("mark job run success", err)
		return
	}

	if s.log != nil {
		s.log.Infof("Job run succeeded run_id=%d hosts=%d actions=%d\n", run.ID, run.TotalHosts, len(actions))
	}

	return
}

// executeAction dispatches an action through the Ansible runner.
func (s *JobService) executeAction(ctx context.Context, action db.JobAction, inventory AnsibleInventory) (output AnsibleRunOutput, err error) {
	var (
		actionCtx context.Context = ctx
		cancel    context.CancelFunc
	)

	if action.TimeoutSeconds > 0 {
		actionCtx, cancel = context.WithTimeout(ctx, time.Duration(action.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	switch action.Type {
	case db.JobActionTypeAnsiblePlaybook:
		output, err = s.ansible.RunPlaybook(actionCtx, AnsibleRunRequest{
			Action:           action,
			InventoryPath:    inventory.Path,
			WorkingDirectory: inventory.WorkDir,
		})
	case db.JobActionTypeShell:
		output, err = s.ansible.RunShellScript(actionCtx, AnsibleRunRequest{
			Action:           action,
			InventoryPath:    inventory.Path,
			WorkingDirectory: inventory.WorkDir,
		})
	default:
		err = NewInvalidInputError("unsupported action type", nil)
	}

	return
}

// failRun marks a run failed and persists the failure reason.
func (s *JobService) failRun(ctx context.Context, run *db.JobRun, runErr error) (err error) {
	run.Status = db.JobRunStatusFailed
	run.EndTime = time.Now().UTC()
	run.FailedHosts = run.TotalHosts
	run.SuccessHosts = 0
	run.Error = runErr.Error()
	run.Summary = "job run failed"

	if err = s.repository.UpdateJobRun(ctx, run); err != nil {
		err = NewPersistenceError("mark job run failed", err)
		return
	}

	if s.log != nil {
		s.log.Errorf("Job run failed run_id=%d status=%d error=%v\n", run.ID, run.Status, runErr)
	}

	err = runErr
	return
}

// createJob validates and persists a job with its actions.
func (s *JobService) createJob(ctx context.Context, input JobInput, protected bool) (job *db.Job, err error) {
	var (
		now    time.Time = time.Now().UTC()
		action db.JobAction
	)
	input = normalizeJobInput(input)

	if err = s.validateJobInput(ctx, input); err != nil {
		return
	}

	job = &db.Job{
		Name:             strings.TrimSpace(input.Name),
		Description:      strings.TrimSpace(input.Description),
		Enabled:          input.Enabled,
		Protected:        protected,
		IntervalSeconds:  0,
		ScheduleType:     input.ScheduleType,
		CronExpr:         strings.TrimSpace(input.CronExpr),
		LongevityType:    normalizeLongevityType(input.LongevityType),
		MaxRuns:          input.MaxRuns,
		DisableAfter:     normalizeDisableAfter(input),
		TargetHostgroups: normalizeStringList(input.TargetHostgroups),
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err = s.repository.InsertJob(ctx, job); err != nil {
		err = NewPersistenceError("insert job", err)
		return
	}

	for position, inputAction := range input.Actions {
		action = db.JobAction{
			JobID:           uint64(job.ID),
			Position:        position + 1,
			Name:            strings.TrimSpace(inputAction.Name),
			Description:     strings.TrimSpace(inputAction.Description),
			Type:            inputAction.Type,
			FilePath:        strings.TrimSpace(inputAction.FilePath),
			Arguments:       normalizeStringList(inputAction.Arguments),
			ContinueOnError: inputAction.ContinueOnError,
			TimeoutSeconds:  inputAction.TimeoutSeconds,
		}

		if err = s.repository.InsertJobAction(ctx, &action); err != nil {
			err = NewPersistenceError("insert job action", err)
			return
		}
	}

	return
}

// validateJobInput checks job fields and enable-time host group requirements.
func (s *JobService) validateJobInput(ctx context.Context, input JobInput) (err error) {
	if err = s.validateJobFields(ctx, input); err != nil {
		return
	}

	if len(input.Actions) == 0 {
		err = NewInvalidInputError("at least one action is required", nil)
		return
	}

	if err = s.validateActions(ctx, input.Actions); err != nil {
		return
	}

	return
}

// validateJobFields checks job fields and enable-time host group requirements.
func (s *JobService) validateJobFields(ctx context.Context, input JobInput) (err error) {
	if strings.TrimSpace(input.Name) == "" {
		err = NewInvalidInputError("job name is required", nil)
		return
	}

	if err = validateSchedule(input); err != nil {
		return
	}

	if err = validateLongevity(input); err != nil {
		return
	}

	if input.Enabled {
		if err = s.validateTargetGroups(ctx, input.TargetHostgroups); err != nil {
			return
		}
	}

	return
}

// validateLongevity checks auto-disable settings.
func validateLongevity(input JobInput) (err error) {
	switch normalizeLongevityType(input.LongevityType) {
	case db.JobLongevityTypePermanent:
	case db.JobLongevityTypeMaxRuns:
		if input.MaxRuns <= 0 {
			err = NewInvalidInputError("run-limited jobs require max_runs", nil)
			return
		}
	case db.JobLongevityTypeUntil:
		if input.DisableAfter.IsZero() {
			err = NewInvalidInputError("date-limited jobs require disable_after", nil)
			return
		}

		if !input.DisableAfter.After(time.Now().UTC()) {
			err = NewInvalidInputError("disable_after must be in the future", nil)
			return
		}
	default:
		err = NewInvalidInputError("unsupported longevity type", nil)
	}

	return
}

// replaceJobActions updates job actions in execution order while preserving run history links.
func (s *JobService) replaceJobActions(ctx context.Context, jobID int, actions []JobActionInput) (err error) {
	var (
		existing []db.JobAction
		action   db.JobAction
	)

	if existing, err = s.repository.ListJobActions(ctx, jobID); err != nil {
		err = NewPersistenceError("list job actions", err)
		return
	}

	for position, inputAction := range actions {
		if position < len(existing) {
			action = existing[position]
		} else {
			action = db.JobAction{JobID: uint64(jobID)}
		}

		action.Position = position + 1
		action.Name = strings.TrimSpace(inputAction.Name)
		action.Description = strings.TrimSpace(inputAction.Description)
		action.Type = inputAction.Type
		action.FilePath = strings.TrimSpace(inputAction.FilePath)
		action.Arguments = normalizeStringList(inputAction.Arguments)
		action.ContinueOnError = inputAction.ContinueOnError
		action.TimeoutSeconds = inputAction.TimeoutSeconds

		if action.ID > 0 {
			if err = s.repository.UpdateJobAction(ctx, &action); err != nil {
				err = NewPersistenceError("update job action", err)
				return
			}

			continue
		}

		if err = s.repository.InsertJobAction(ctx, &action); err != nil {
			err = NewPersistenceError("insert job action", err)
			return
		}
	}

	if len(existing) > len(actions) {
		for _, action = range existing[len(actions):] {
			if err = s.repository.DeleteJobAction(ctx, action.ID); err != nil {
				err = NewPersistenceError("delete job action", err)
				return
			}
		}
	}

	return
}

// validateSchedule checks schedule-specific required fields.
func validateSchedule(input JobInput) (err error) {
	switch input.ScheduleType {
	case db.ScheduleTypeCron:
		if strings.TrimSpace(input.CronExpr) == "" {
			err = NewInvalidInputError("cron jobs require cron_expr", nil)
			return
		}
		if _, err = parseCronSchedule(input.CronExpr); err != nil {
			err = NewInvalidInputError(fmt.Sprintf("invalid cron expression: %v", err), nil)
			return
		}
	case db.ScheduleTypeManual:
	default:
		err = NewInvalidInputError("unsupported schedule type", nil)
	}

	return
}

func normalizeJobInput(input JobInput) (normalized JobInput) {
	normalized = input
	if normalized.ScheduleType == db.ScheduleTypeInterval {
		normalized.ScheduleType = db.ScheduleTypeCron
		normalized.CronExpr = intervalSecondsToCron(normalized.IntervalSeconds)
	}

	if normalized.ScheduleType == db.ScheduleTypeCron {
		normalized.IntervalSeconds = 0
	}

	return
}

// validateActions checks supported action definitions.
func (s *JobService) validateActions(ctx context.Context, actions []JobActionInput) (err error) {
	for _, action := range actions {
		switch action.Type {
		case db.JobActionTypeAnsiblePlaybook, db.JobActionTypeShell:
		default:
			err = NewInvalidInputError("unsupported action type", nil)
			return
		}

		if strings.TrimSpace(action.Name) == "" {
			err = NewInvalidInputError("action name is required", nil)
			return
		}

		if strings.TrimSpace(action.FilePath) == "" {
			err = NewInvalidInputError("action file is required", nil)
			return
		}

		if err = s.files.ValidateActionFile(ctx, action.FilePath, action.Type); err != nil {
			return
		}
	}

	return
}

// validateTargetGroups checks selected host groups against FreeIPA.
func (s *JobService) validateTargetGroups(ctx context.Context, groups []string) (err error) {
	var (
		selected []string = normalizeStringList(groups)
		valid    []string
		validSet map[string]struct{}
		ok       bool
	)

	if len(selected) == 0 {
		err = NewInvalidInputError("enabled jobs require at least one target host group", nil)
		return
	}

	if valid, err = s.groups.GetHostGroups(ctx); err != nil {
		err = NewExecutionError("load host groups", err)
		return
	}

	validSet = buildStringSet(valid)
	for _, group := range selected {
		if _, ok = validSet[strings.ToLower(group)]; !ok {
			err = NewInvalidInputError(fmt.Sprintf("unknown host group %q", group), nil)
			return
		}
	}

	return
}

// defaultJobInputs returns protected built-in job definitions.
func defaultJobInputs() (jobs []JobInput) {
	jobs = []JobInput{
		{
			Name:             "Health",
			Description:      "For online systems, checks usage and system health, then reports major warnings.",
			Enabled:          false,
			ScheduleType:     db.ScheduleTypeCron,
			CronExpr:         "0 */5 * * * *",
			LongevityType:    db.JobLongevityTypePermanent,
			TargetHostgroups: []string{},
			Actions: []JobActionInput{
				defaultAnsibleAction("Health playbook", "playbooks/health.yml"),
			},
		},
		{
			Name:             "Heartbeat",
			Description:      "Checks login to online systems and enumerates uptime and online users.",
			Enabled:          false,
			ScheduleType:     db.ScheduleTypeCron,
			CronExpr:         "0 * * * * *",
			LongevityType:    db.JobLongevityTypePermanent,
			TargetHostgroups: []string{},
			Actions: []JobActionInput{
				defaultAnsibleAction("Heartbeat playbook", "playbooks/heartbeat.yml"),
			},
		},
		{
			Name:             "Inventory",
			Description:      "Once a week, poll hardware and operating system information.",
			Enabled:          false,
			ScheduleType:     db.ScheduleTypeCron,
			CronExpr:         "0 0 * * 0",
			LongevityType:    db.JobLongevityTypePermanent,
			TargetHostgroups: []string{},
			Actions: []JobActionInput{
				defaultAnsibleAction("Inventory playbook", "playbooks/inventory.yml"),
			},
		},
		{
			Name:             "Software Update",
			Description:      "Every Monday at 5am local time, apply package updates and upgrades.",
			Enabled:          false,
			ScheduleType:     db.ScheduleTypeCron,
			CronExpr:         "0 5 * * 1",
			LongevityType:    db.JobLongevityTypePermanent,
			TargetHostgroups: []string{},
			Actions: []JobActionInput{
				defaultAnsibleAction("Software update playbook", "playbooks/software-update.yml"),
			},
		},
	}
	return
}

// normalizeLongevityType maps unset longevity to permanent.
func normalizeLongevityType(longevityType db.JobLongevityType) (normalized db.JobLongevityType) {
	normalized = longevityType
	if normalized == db.JobLongevityTypeUnknown {
		normalized = db.JobLongevityTypePermanent
	}

	return
}

// isTerminalJobRunStatus reports run states that should never execute again.
func isTerminalJobRunStatus(status db.JobRunStatus) (terminal bool) {
	terminal = status == db.JobRunStatusSuccess || status == db.JobRunStatusFailed
	return
}

// normalizeDisableAfter keeps date limits only when the job uses them.
func normalizeDisableAfter(input JobInput) (disableAfter time.Time) {
	if normalizeLongevityType(input.LongevityType) == db.JobLongevityTypeUntil {
		disableAfter = input.DisableAfter.UTC()
	}

	return
}

// defaultAnsibleAction returns a protected Ansible playbook action.
func defaultAnsibleAction(name string, playbook string) (input JobActionInput) {
	input = JobActionInput{
		Name:           name,
		Type:           db.JobActionTypeAnsiblePlaybook,
		FilePath:       playbook,
		Arguments:      []string{},
		TimeoutSeconds: 600,
	}
	return
}

// normalizeStringList trims and removes empty strings.
func normalizeStringList(items []string) (normalized []string) {
	normalized = make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			normalized = append(normalized, item)
		}
	}

	return
}

// buildStringSet creates a case-insensitive string lookup set.
func buildStringSet(items []string) (set map[string]struct{}) {
	set = make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			set[item] = struct{}{}
		}
	}

	return
}

type DefaultActionFactory struct{}

// NewDefaultActionFactory creates the default action factory.
func NewDefaultActionFactory() (factory *DefaultActionFactory) {
	factory = &DefaultActionFactory{}
	return
}

// NewAction creates an executable action wrapper.
func (f *DefaultActionFactory) NewAction(action db.JobAction) (executable Action, err error) {
	switch action.Type {
	case db.JobActionTypeAnsiblePlaybook:
		executable = AnsiblePlaybookAction{action: action}
	case db.JobActionTypeShell:
		executable = ShellAction{action: action}
	default:
		err = fmt.Errorf("unsupported job action type %d", action.Type)
	}

	return
}

type AnsiblePlaybookAction struct {
	action db.JobAction
}

// Execute validates context for a future Ansible playbook action.
func (a AnsiblePlaybookAction) Execute(ctx context.Context, run *db.JobRun) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	return
}

type ShellAction struct {
	action db.JobAction
}

// Execute validates context for a future shell action.
func (a ShellAction) Execute(ctx context.Context, run *db.JobRun) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	return
}
