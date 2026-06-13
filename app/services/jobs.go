package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/z46-dev/overlord-ipa/db"
)

type JobRepository interface {
	ListJobs(ctx context.Context) ([]db.Job, error)
	GetJob(ctx context.Context, id int) (*db.Job, error)
	GetJobByName(ctx context.Context, name string) (*db.Job, error)
	InsertJob(ctx context.Context, job *db.Job) error
	UpdateJob(ctx context.Context, job *db.Job) error
	InsertJobAction(ctx context.Context, action *db.JobAction) error
	DeleteJobActions(ctx context.Context, jobID int) error
	ListJobActions(ctx context.Context, jobID int) ([]db.JobAction, error)
}

type HostGroupProvider interface {
	GetHostGroups(ctx context.Context) ([]string, error)
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
	groups     HostGroupProvider
	files      ActionFileValidator
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
	Name             string           `json:"name"`
	Description      string           `json:"description"`
	Enabled          bool             `json:"enabled"`
	IntervalSeconds  int64            `json:"interval_seconds"`
	ScheduleType     db.ScheduleType  `json:"schedule_type"`
	CronExpr         string           `json:"cron_expr"`
	TargetHostgroups []string         `json:"target_hostgroups"`
	Actions          []JobActionInput `json:"actions"`
}

// NewJobService creates the job business service.
func NewJobService(repository JobRepository, actions ActionFactory, groups HostGroupProvider, files ActionFileValidator) (service *JobService) {
	service = &JobService{
		repository: repository,
		actions:    actions,
		groups:     groups,
		files:      files,
	}
	return
}

// EnsureDefaultJobs creates protected built-in job examples if missing.
func (s *JobService) EnsureDefaultJobs(ctx context.Context) (err error) {
	var defaults []JobInput = defaultJobInputs()
	var existing *db.Job

	for _, input := range defaults {
		if existing, err = s.repository.GetJobByName(ctx, input.Name); err != nil {
			err = NewPersistenceError("get default job", err)
			return
		}

		if existing != nil {
			continue
		}

		if _, err = s.createJob(ctx, input, true); err != nil {
			return
		}
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

// CreateJob validates and persists a new custom job.
func (s *JobService) CreateJob(ctx context.Context, input JobInput) (job *db.Job, err error) {
	job, err = s.createJob(ctx, input, false)
	return
}

// UpdateJob validates and persists an existing job.
func (s *JobService) UpdateJob(ctx context.Context, jobID int, input JobInput) (job *db.Job, err error) {
	var existing *db.Job

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
	existing.IntervalSeconds = input.IntervalSeconds
	existing.ScheduleType = input.ScheduleType
	existing.CronExpr = strings.TrimSpace(input.CronExpr)
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

// RunJob runs a job through the configured action framework.
func (s *JobService) RunJob(ctx context.Context, jobID int, triggeredBy string) (run *db.JobRun, err error) {
	var (
		job        *db.Job
		actions    []db.JobAction
		executable Action
	)

	if job, err = s.repository.GetJob(ctx, jobID); err != nil {
		err = NewPersistenceError("get job", err)
		return
	}

	if job == nil {
		err = NewNotFoundError("job not found", nil)
		return
	}

	if actions, err = s.repository.ListJobActions(ctx, job.ID); err != nil {
		err = NewPersistenceError("list job actions", err)
		return
	}

	run = &db.JobRun{
		JobID:            uint64(job.ID),
		Status:           db.JobRunStatusRunning,
		TriggerType:      db.JobRunTriggerTypeAPI,
		TriggeredBy:      triggeredBy,
		StartTime:        time.Now().UTC(),
		TargetHostgroups: job.TargetHostgroups,
	}

	for _, action := range actions {
		if executable, err = s.actions.NewAction(action); err != nil {
			run.Status = db.JobRunStatusFailed
			run.Error = err.Error()
			run.EndTime = time.Now().UTC()
			err = NewExecutionError("create action", err)
			return
		}

		if err = executable.Execute(ctx, run); err != nil {
			run.Status = db.JobRunStatusFailed
			run.Error = err.Error()
			run.EndTime = time.Now().UTC()
			err = NewExecutionError("execute action", err)
			return
		}
	}

	run.Status = db.JobRunStatusSuccess
	run.EndTime = time.Now().UTC()
	run.Summary = "mock execution completed"
	return
}

// createJob validates and persists a job with its actions.
func (s *JobService) createJob(ctx context.Context, input JobInput, protected bool) (job *db.Job, err error) {
	var (
		now    time.Time = time.Now().UTC()
		action db.JobAction
	)

	if err = s.validateJobInput(ctx, input); err != nil {
		return
	}

	job = &db.Job{
		Name:             strings.TrimSpace(input.Name),
		Description:      strings.TrimSpace(input.Description),
		Enabled:          input.Enabled,
		Protected:        protected,
		IntervalSeconds:  input.IntervalSeconds,
		ScheduleType:     input.ScheduleType,
		CronExpr:         strings.TrimSpace(input.CronExpr),
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

	if input.Enabled {
		if err = s.validateTargetGroups(ctx, input.TargetHostgroups); err != nil {
			return
		}
	}

	return
}

// replaceJobActions replaces custom job actions in execution order.
func (s *JobService) replaceJobActions(ctx context.Context, jobID int, actions []JobActionInput) (err error) {
	var action db.JobAction

	if err = s.repository.DeleteJobActions(ctx, jobID); err != nil {
		err = NewPersistenceError("delete job actions", err)
		return
	}

	for position, inputAction := range actions {
		action = db.JobAction{
			JobID:           uint64(jobID),
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

// validateSchedule checks schedule-specific required fields.
func validateSchedule(input JobInput) (err error) {
	switch input.ScheduleType {
	case db.ScheduleTypeInterval:
		if input.IntervalSeconds <= 0 {
			err = NewInvalidInputError("interval jobs require interval_seconds", nil)
			return
		}
	case db.ScheduleTypeCron:
		if strings.TrimSpace(input.CronExpr) == "" {
			err = NewInvalidInputError("cron jobs require cron_expr", nil)
			return
		}
	case db.ScheduleTypeManual:
	default:
		err = NewInvalidInputError("unsupported schedule type", nil)
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
			Description:      "Every minute, check SSH connectivity and poll usage, uptime, and users.",
			Enabled:          false,
			IntervalSeconds:  60,
			ScheduleType:     db.ScheduleTypeInterval,
			TargetHostgroups: []string{},
			Actions: []JobActionInput{
				defaultAnsibleAction("Health playbook", "playbooks/health.yml"),
			},
		},
		{
			Name:             "Inventory",
			Description:      "Once a week, poll hardware and operating system information.",
			Enabled:          false,
			ScheduleType:     db.ScheduleTypeCron,
			CronExpr:         "0 0 * * 0",
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
			TargetHostgroups: []string{},
			Actions: []JobActionInput{
				defaultAnsibleAction("Software update playbook", "playbooks/software-update.yml"),
			},
		},
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
