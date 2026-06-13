package services

import (
	"context"

	"github.com/z46-dev/overlord-ipa/db"
)

type DashboardRepository interface {
	CountHosts(ctx context.Context) (int64, error)
	CountJobs(ctx context.Context) (int64, error)
	CountJobRunsByStatus(ctx context.Context, status db.JobRunStatus) (int64, error)
	ListRecentJobRuns(ctx context.Context, limit int) ([]db.JobRun, error)
}

type DashboardSummary struct {
	Hosts       int64       `json:"hosts"`
	Jobs        int64       `json:"jobs"`
	QueuedJobs  int64       `json:"queued_jobs"`
	RunningJobs int64       `json:"running_jobs"`
	FailedJobs  int64       `json:"failed_jobs"`
	RecentRuns  []db.JobRun `json:"recent_runs"`
}

type DashboardService struct {
	repository DashboardRepository
}

// NewDashboardService creates the dashboard business service.
func NewDashboardService(repository DashboardRepository) (service *DashboardService) {
	service = &DashboardService{repository: repository}
	return
}

// Summary returns dashboard aggregate counters.
func (s *DashboardService) Summary(ctx context.Context) (summary DashboardSummary, err error) {
	var (
		hosts       int64
		jobs        int64
		queuedJobs  int64
		runningJobs int64
		failedJobs  int64
		recentRuns  []db.JobRun
	)

	if hosts, err = s.repository.CountHosts(ctx); err != nil {
		err = NewPersistenceError("count hosts", err)
		return
	}

	if jobs, err = s.repository.CountJobs(ctx); err != nil {
		err = NewPersistenceError("count jobs", err)
		return
	}

	if queuedJobs, err = s.repository.CountJobRunsByStatus(ctx, db.JobRunStatusQueued); err != nil {
		err = NewPersistenceError("count queued jobs", err)
		return
	}

	if runningJobs, err = s.repository.CountJobRunsByStatus(ctx, db.JobRunStatusRunning); err != nil {
		err = NewPersistenceError("count running jobs", err)
		return
	}

	if failedJobs, err = s.repository.CountJobRunsByStatus(ctx, db.JobRunStatusFailed); err != nil {
		err = NewPersistenceError("count failed jobs", err)
		return
	}

	if recentRuns, err = s.repository.ListRecentJobRuns(ctx, 8); err != nil {
		err = NewPersistenceError("list recent job runs", err)
		return
	}

	summary = DashboardSummary{
		Hosts:       hosts,
		Jobs:        jobs,
		QueuedJobs:  queuedJobs,
		RunningJobs: runningJobs,
		FailedJobs:  failedJobs,
		RecentRuns:  recentRuns,
	}
	return
}
