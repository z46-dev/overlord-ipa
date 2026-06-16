package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/z46-dev/golog"
	"github.com/z46-dev/overlord-ipa/db"
)

type SchedulerRepository interface {
	ListEnabledJobs(ctx context.Context) ([]db.Job, error)
	UpdateJob(ctx context.Context, job *db.Job) error
	CountJobRunsByJobID(ctx context.Context, jobID int) (int64, error)
	CountActiveJobRunsByJobID(ctx context.Context, jobID int) (int64, error)
}

type ScheduledJobRunner interface {
	RunScheduledJob(ctx context.Context, jobID int) (*db.JobRun, error)
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
	NextRunAt       time.Time           `json:"next_run_at"`
}

type scheduledEntry struct {
	job      ScheduledJob
	schedule cron.Schedule
}

type SchedulerSnapshot struct {
	LoadedJobs []ScheduledJob `json:"loaded_jobs"`
}

type Scheduler struct {
	repository SchedulerRepository
	runner     ScheduledJobRunner
	log        *golog.Logger
	mu         sync.RWMutex
	jobs       []scheduledEntry
}

// NewScheduler creates a cron-backed scheduler service.
func NewScheduler(repository SchedulerRepository, runner ScheduledJobRunner, logger *golog.Logger) (scheduler *Scheduler) {
	scheduler = &Scheduler{
		repository: repository,
		runner:     runner,
		log:        serviceLogger(logger, "[SCHEDULER]", golog.BoldPurple),
	}
	return
}

// Load reads enabled cron jobs into the scheduler.
func (s *Scheduler) Load(ctx context.Context) (err error) {
	var (
		jobs       []db.Job
		scheduled  []scheduledEntry
		disableJob bool
		expr       string
		schedule   cron.Schedule
		now        time.Time = time.Now().UTC()
	)

	if jobs, err = s.repository.ListEnabledJobs(ctx); err != nil {
		err = NewPersistenceError("load enabled jobs", err)
		return
	}

	scheduled = make([]scheduledEntry, 0, len(jobs))
	for _, job := range jobs {
		if disableJob, _, err = s.shouldDisable(ctx, job); err != nil {
			return
		}

		if disableJob {
			job.Enabled = false
			job.UpdatedAt = now
			if err = s.repository.UpdateJob(ctx, &job); err != nil {
				err = NewPersistenceError("disable expired job", err)
				return
			}

			continue
		}

		if job.ScheduleType == db.ScheduleTypeInterval {
			expr = intervalSecondsToCron(job.IntervalSeconds)
		} else {
			expr = strings.TrimSpace(job.CronExpr)
		}

		if job.ScheduleType == db.ScheduleTypeManual || expr == "" {
			continue
		}

		if schedule, err = parseCronSchedule(expr); err != nil {
			if s.log != nil {
				s.log.Errorf("Skipping job with invalid cron job_id=%d name=%s cron=%q error=%v\n", job.ID, job.Name, expr, err)
			}
			continue
		}

		scheduled = append(scheduled, scheduledEntry{
			job: ScheduledJob{
				ID:              job.ID,
				Name:            job.Name,
				ScheduleType:    db.ScheduleTypeCron,
				IntervalSeconds: 0,
				CronExpr:        expr,
				LongevityType:   normalizeScheduledLongevity(job.LongevityType),
				MaxRuns:         job.MaxRuns,
				DisableAfter:    job.DisableAfter,
				Enabled:         job.Enabled,
				NextRunAt:       schedule.Next(now),
			},
			schedule: schedule,
		})
	}

	s.mu.Lock()
	s.jobs = scheduled
	s.mu.Unlock()

	if s.log != nil {
		s.log.Infof("Loaded %d scheduled job(s)\n", len(scheduled))
	}
	return
}

// Run starts the scheduler loop until the context is canceled.
func (s *Scheduler) Run(ctx context.Context) (err error) {
	var ticker *time.Ticker = time.NewTicker(time.Second)
	defer ticker.Stop()

	if s.log != nil {
		s.log.Info("Starting scheduler\n")
	}

	for {
		select {
		case <-ctx.Done():
			if s.log != nil {
				s.log.Info("Stopped scheduler\n")
			}
			err = ctx.Err()
			return
		case now := <-ticker.C:
			s.runDue(ctx, now.UTC())
		}
	}
}

func (s *Scheduler) runDue(ctx context.Context, now time.Time) {
	var due []scheduledEntry

	s.mu.RLock()
	for _, entry := range s.jobs {
		if !entry.job.NextRunAt.IsZero() && !entry.job.NextRunAt.After(now) {
			due = append(due, entry)
		}
	}
	s.mu.RUnlock()

	for _, entry := range due {
		s.runOne(ctx, entry, now)
	}
}

func (s *Scheduler) runOne(ctx context.Context, entry scheduledEntry, now time.Time) {
	var (
		active int64
		err    error
	)

	if active, err = s.repository.CountActiveJobRunsByJobID(ctx, entry.job.ID); err != nil {
		if s.log != nil {
			s.log.Errorf("Unable to check active runs job_id=%d: %v\n", entry.job.ID, err)
		}
		s.advance(entry.job.ID, entry.schedule, now)
		return
	}

	if active > 0 {
		if s.log != nil {
			s.log.Infof("Skipping scheduled job with active run job_id=%d name=%s active=%d\n", entry.job.ID, entry.job.Name, active)
		}
		s.advance(entry.job.ID, entry.schedule, now)
		return
	}

	if s.runner == nil {
		if s.log != nil {
			s.log.Errorf("Scheduler runner is not configured job_id=%d\n", entry.job.ID)
		}
		s.advance(entry.job.ID, entry.schedule, now)
		return
	}

	if _, err = s.runner.RunScheduledJob(ctx, entry.job.ID); err != nil {
		if s.log != nil {
			s.log.Errorf("Scheduled job enqueue failed job_id=%d name=%s: %v\n", entry.job.ID, entry.job.Name, err)
		}
		s.advance(entry.job.ID, entry.schedule, now)
		return
	}

	if s.log != nil {
		s.log.Infof("Scheduled job enqueued job_id=%d name=%s\n", entry.job.ID, entry.job.Name)
	}

	if err = s.Load(ctx); err != nil {
		if s.log != nil {
			s.log.Errorf("Scheduler reload failed after enqueue job_id=%d: %v\n", entry.job.ID, err)
		}
		s.advance(entry.job.ID, entry.schedule, now)
	}
}

func (s *Scheduler) advance(jobID int, schedule cron.Schedule, now time.Time) {
	nextRunAt := schedule.Next(now)

	s.mu.Lock()
	defer s.mu.Unlock()

	for index := range s.jobs {
		if s.jobs[index].job.ID == jobID {
			s.jobs[index].job.NextRunAt = nextRunAt
			return
		}
	}
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

	jobs := make([]ScheduledJob, 0, len(s.jobs))
	for _, entry := range s.jobs {
		jobs = append(jobs, entry.job)
	}
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

func parseCronSchedule(expression string) (schedule cron.Schedule, err error) {
	parser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	return parser.Parse(strings.TrimSpace(expression))
}

func intervalSecondsToCron(seconds int64) string {
	switch {
	case seconds <= 0:
		return ""
	case seconds < 60:
		return fmt.Sprintf("*/%d * * * * *", seconds)
	case seconds%3600 == 0:
		return fmt.Sprintf("0 0 */%d * * *", seconds/3600)
	case seconds%60 == 0:
		return fmt.Sprintf("0 */%d * * * *", seconds/60)
	default:
		return fmt.Sprintf("*/%d * * * * *", seconds)
	}
}
