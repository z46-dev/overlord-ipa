package db

import (
	"context"
	"fmt"

	"github.com/z46-dev/gosqlite"
)

type Repository struct{}

// NewRepository creates a database-backed repository.
func NewRepository() (repo *Repository) {
	repo = &Repository{}
	return
}

// CountHosts returns the total number of persisted hosts.
func (r *Repository) CountHosts(ctx context.Context) (count int64, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if count, err = Hosts.Count(); err != nil {
		return
	}

	return
}

// CountJobs returns the total number of persisted jobs.
func (r *Repository) CountJobs(ctx context.Context) (count int64, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if count, err = Jobs.Count(); err != nil {
		return
	}

	return
}

// CountJobRunsByStatus returns the number of job runs matching a status.
func (r *Repository) CountJobRunsByStatus(ctx context.Context, status JobRunStatus) (count int64, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if count, err = JobRuns.CountWithFilter(
		gosqlite.NewFilter().KeyCmp(JobRuns.FieldByGoName("Status"), gosqlite.OpEqual, status),
	); err != nil {
		return
	}

	return
}

// ListEnabledJobs returns all enabled jobs.
func (r *Repository) ListEnabledJobs(ctx context.Context) (jobs []Job, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	var rows []*Job
	if rows, err = Jobs.SelectAllWithFilter(
		gosqlite.NewFilter().KeyCmp(Jobs.FieldByGoName("Enabled"), gosqlite.OpEqual, true),
	); err != nil {
		return
	}

	jobs = dereference(rows)
	return
}

// ListJobs returns all persisted jobs.
func (r *Repository) ListJobs(ctx context.Context) (jobs []Job, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	var rows []*Job
	if rows, err = Jobs.SelectAll(); err != nil {
		return
	}

	jobs = dereference(rows)
	return
}

// GetJob returns a single job by primary key.
func (r *Repository) GetJob(ctx context.Context, id int) (job *Job, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if job, err = Jobs.Select(id); err != nil {
		return
	}

	return
}

// GetJobByName returns a single job by unique name.
func (r *Repository) GetJobByName(ctx context.Context, name string) (job *Job, err error) {
	var rows []*Job

	if err = ctx.Err(); err != nil {
		return
	}

	if rows, err = Jobs.SelectAllWithFilter(
		gosqlite.NewFilter().
			KeyCmp(Jobs.FieldByGoName("Name"), gosqlite.OpEqual, name).
			Limit(1),
	); err != nil {
		return
	}

	if len(rows) > 0 {
		job = rows[0]
	}

	return
}

// InsertJob persists a new job.
func (r *Repository) InsertJob(ctx context.Context, job *Job) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if err = Jobs.Insert(job); err != nil {
		return
	}

	return
}

// UpdateJob persists an existing job.
func (r *Repository) UpdateJob(ctx context.Context, job *Job) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if err = Jobs.Update(job); err != nil {
		return
	}

	return
}

// InsertJobAction persists a new job action.
func (r *Repository) InsertJobAction(ctx context.Context, action *JobAction) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if err = JobActions.Insert(action); err != nil {
		return
	}

	return
}

// DeleteJobActions removes all actions for a job.
func (r *Repository) DeleteJobActions(ctx context.Context, jobID int) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if jobID <= 0 {
		err = fmt.Errorf("job id must be positive")
		return
	}

	if _, err = JobActions.DeleteWithFilter(
		gosqlite.NewFilter().KeyCmp(JobActions.FieldByGoName("JobID"), gosqlite.OpEqual, uint64(jobID)),
	); err != nil {
		return
	}

	return
}

// ListJobActions returns all actions for a job in execution order.
func (r *Repository) ListJobActions(ctx context.Context, jobID int) (actions []JobAction, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if jobID <= 0 {
		err = fmt.Errorf("job id must be positive")
		return
	}

	var rows []*JobAction
	if rows, err = JobActions.SelectAllWithFilter(
		gosqlite.NewFilter().
			KeyCmp(JobActions.FieldByGoName("JobID"), gosqlite.OpEqual, uint64(jobID)).
			Ordering(JobActions.FieldByGoName("Position"), true),
	); err != nil {
		return
	}

	actions = dereference(rows)
	return
}

// dereference converts non-nil pointers into values.
func dereference[T any](items []*T) (values []T) {
	values = make([]T, 0, len(items))
	for _, item := range items {
		if item != nil {
			values = append(values, *item)
		}
	}

	return
}
