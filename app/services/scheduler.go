package services

import (
	"context"
	"sync"
	"time"

	"github.com/z46-dev/overlord-ipa/db"
)

type SchedulerRepository interface {
	ListEnabledJobs(ctx context.Context) ([]db.Job, error)
	UpdateJob(ctx context.Context, job *db.Job) error
	CountJobRunsByJobID(ctx context.Context, jobID int) (int64, error)
}

type Executor interface {
	RunJob(job db.Job) error
}

type MockExecutor struct{}

// NewMockExecutor creates a placeholder executor.
func NewMockExecutor() (executor *MockExecutor) {
	executor = &MockExecutor{}
	return
}

// RunJob accepts a scheduled job without executing it yet.
func (e *MockExecutor) RunJob(job db.Job) (err error) {
	return
}

type ScheduledJob struct {
	ID              int                 `json:"id"`
	Name            string              `json:"name"`
	ScheduleType    db.ScheduleType     `json:"schedule_type"`
	IntervalSeconds int64               `json:"interval_seconds"`
	CronExpr        string              `json:"cron_expr"`
	LongevityType   db.JobLongevityType `json:"longevity_type"`
	MaxRuns         int                 `json:"max_runs"`
	DisableAfter    time.Time           `json:"disable_after"`
	Enabled         bool                `json:"enabled"`
}

type SchedulerSnapshot struct {
	LoadedJobs []ScheduledJob `json:"loaded_jobs"`
}

type Scheduler struct {
	repository SchedulerRepository
	executor   Executor
	mu         sync.RWMutex
	jobs       []ScheduledJob
}

// NewScheduler creates a scheduler service.
func NewScheduler(repository SchedulerRepository, executor Executor) (scheduler *Scheduler) {
	scheduler = &Scheduler{
		repository: repository,
		executor:   executor,
	}
	return
}

// Load reads enabled jobs into the scheduler snapshot.
func (s *Scheduler) Load(ctx context.Context) (err error) {
	var (
		jobs       []db.Job
		scheduled  []ScheduledJob
		disableJob bool
	)

	if jobs, err = s.repository.ListEnabledJobs(ctx); err != nil {
		err = NewPersistenceError("load enabled jobs", err)
		return
	}

	scheduled = make([]ScheduledJob, 0, len(jobs))
	for _, job := range jobs {
		if disableJob, _, err = s.shouldDisable(ctx, job); err != nil {
			return
		}

		if disableJob {
			job.Enabled = false
			job.UpdatedAt = time.Now().UTC()
			if err = s.repository.UpdateJob(ctx, &job); err != nil {
				err = NewPersistenceError("disable expired job", err)
				return
			}

			continue
		}

		if !isSchedulable(job) {
			continue
		}

		scheduled = append(scheduled, ScheduledJob{
			ID:              job.ID,
			Name:            job.Name,
			ScheduleType:    job.ScheduleType,
			IntervalSeconds: job.IntervalSeconds,
			CronExpr:        job.CronExpr,
			LongevityType:   normalizeScheduledLongevity(job.LongevityType),
			MaxRuns:         job.MaxRuns,
			DisableAfter:    job.DisableAfter,
			Enabled:         job.Enabled,
		})
	}

	s.mu.Lock()
	s.jobs = scheduled
	s.mu.Unlock()
	return
}

// shouldDisable reports whether a job reached its longevity limit.
func (s *Scheduler) shouldDisable(ctx context.Context, job db.Job) (disable bool, runCount int64, err error) {
	switch normalizeScheduledLongevity(job.LongevityType) {
	case db.JobLongevityTypePermanent:
	case db.JobLongevityTypeMaxRuns:
		if job.MaxRuns <= 0 {
			return
		}

		if runCount, err = s.repository.CountJobRunsByJobID(ctx, job.ID); err != nil {
			err = NewPersistenceError("count job runs", err)
			return
		}

		disable = runCount >= int64(job.MaxRuns)
	case db.JobLongevityTypeUntil:
		if !job.DisableAfter.IsZero() && !job.DisableAfter.After(time.Now().UTC()) {
			disable = true
		}
	}

	return
}

// Snapshot returns the currently loaded scheduler state.
func (s *Scheduler) Snapshot() (snapshot SchedulerSnapshot) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var jobs []ScheduledJob = make([]ScheduledJob, len(s.jobs))
	copy(jobs, s.jobs)
	snapshot = SchedulerSnapshot{LoadedJobs: jobs}
	return
}

// normalizeScheduledLongevity maps unset longevity to permanent.
func normalizeScheduledLongevity(longevityType db.JobLongevityType) (normalized db.JobLongevityType) {
	normalized = longevityType
	if normalized == db.JobLongevityTypeUnknown {
		normalized = db.JobLongevityTypePermanent
	}

	return
}

// isSchedulable reports whether a job has enough schedule data.
func isSchedulable(job db.Job) (ok bool) {
	switch job.ScheduleType {
	case db.ScheduleTypeInterval:
		ok = job.IntervalSeconds > 0
	case db.ScheduleTypeCron:
		ok = job.CronExpr != ""
	case db.ScheduleTypeManual:
		ok = true
	default:
		ok = false
	}

	return
}
