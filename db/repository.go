package db

import (
	"context"
	"fmt"
	"slices"
	"strings"

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

// CountJobRunsByJobID returns the number of runs for a job.
func (r *Repository) CountJobRunsByJobID(ctx context.Context, jobID int) (count int64, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if jobID <= 0 {
		err = fmt.Errorf("job id must be positive")
		return
	}

	if count, err = JobRuns.CountWithFilter(
		gosqlite.NewFilter().KeyCmp(JobRuns.FieldByGoName("JobID"), gosqlite.OpEqual, uint64(jobID)),
	); err != nil {
		return
	}

	return
}

// CountActiveJobRunsByJobID returns queued or running runs for a job.
func (r *Repository) CountActiveJobRunsByJobID(ctx context.Context, jobID int) (count int64, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if jobID <= 0 {
		err = fmt.Errorf("job id must be positive")
		return
	}

	if count, err = JobRuns.CountWithFilter(
		gosqlite.NewFilter().
			KeyCmp(JobRuns.FieldByGoName("JobID"), gosqlite.OpEqual, uint64(jobID)).
			And().
			KeyCmp(JobRuns.FieldByGoName("Status"), gosqlite.OpIn, []JobRunStatus{JobRunStatusQueued, JobRunStatusRunning}),
	); err != nil {
		return
	}

	return
}

// GetHostByFQDN returns a host inventory row by FQDN.
func (r *Repository) GetHostByFQDN(ctx context.Context, fqdn string) (host *Host, err error) {
	var rows []*Host

	if err = ctx.Err(); err != nil {
		return
	}

	if fqdn == "" {
		return
	}

	if rows, err = Hosts.SelectAllWithFilter(
		gosqlite.NewFilter().
			KeyCmp(Hosts.FieldByGoName("FQDN"), gosqlite.OpEqual, fqdn).
			Limit(1),
	); err != nil {
		return
	}

	if len(rows) > 0 {
		host = rows[0]
	}

	return
}

// UpsertHost inserts or updates host inventory data by FQDN.
func (r *Repository) UpsertHost(ctx context.Context, host *Host) (err error) {
	var existing *Host

	if err = ctx.Err(); err != nil {
		return
	}

	if host == nil {
		err = fmt.Errorf("host is required")
		return
	}

	if host.FQDN == "" {
		err = fmt.Errorf("host fqdn is required")
		return
	}

	if existing, err = r.GetHostByFQDN(ctx, host.FQDN); err != nil {
		return
	}

	if existing == nil {
		if err = Hosts.Insert(host); err != nil {
			return
		}

		return
	}

	host.ID = existing.ID
	if host.IPAHostDN == "" {
		host.IPAHostDN = existing.IPAHostDN
	}

	if len(host.Hostgroups) == 0 {
		host.Hostgroups = existing.Hostgroups
	}

	if host.OSName == "" {
		host.OSName = existing.OSName
	}

	if host.OSVersion == "" {
		host.OSVersion = existing.OSVersion
	}

	if host.Arch == "" {
		host.Arch = existing.Arch
	}

	if host.Kernel == "" {
		host.Kernel = existing.Kernel
	}

	if host.AgentVersion == "" {
		host.AgentVersion = existing.AgentVersion
	}

	if len(host.NetworkAddresses) == 0 {
		host.NetworkAddresses = existing.NetworkAddresses
	}

	if host.LastInventoryAt.IsZero() {
		host.LastInventoryAt = existing.LastInventoryAt
	}

	if host.LastHealthAt.IsZero() {
		host.LastHealthAt = existing.LastHealthAt
	}

	if host.LastUpdateAt.IsZero() {
		host.LastUpdateAt = existing.LastUpdateAt
	}

	if host.CreatedAt.IsZero() {
		host.CreatedAt = existing.CreatedAt
	}

	if host.UpdatedAt.IsZero() {
		host.UpdatedAt = existing.UpdatedAt
	}

	if host.ProcessorModel == "" {
		host.ProcessorModel = existing.ProcessorModel
	}

	if host.ProcessorCount == 0 {
		host.ProcessorCount = existing.ProcessorCount
	}

	if host.ProcessorCores == 0 {
		host.ProcessorCores = existing.ProcessorCores
	}

	if host.ProcessorThreads == 0 {
		host.ProcessorThreads = existing.ProcessorThreads
	}

	if host.MemoryMB == 0 {
		host.MemoryMB = existing.MemoryMB
	}

	if len(host.Disks) == 0 {
		host.Disks = existing.Disks
	}

	if err = Hosts.Update(host); err != nil {
		return
	}

	return
}

// InsertJobHostResult persists one per-host job result.
func (r *Repository) InsertJobHostResult(ctx context.Context, result *JobHostResult) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if result == nil {
		err = fmt.Errorf("job host result is required")
		return
	}

	if err = JobHostResults.Insert(result); err != nil {
		return
	}

	return
}

// ListRecentJobRuns returns recent job runs in newest-first order.
func (r *Repository) ListRecentJobRuns(ctx context.Context, limit int) (runs []JobRun, err error) {
	var rows []*JobRun

	if err = ctx.Err(); err != nil {
		return
	}

	if limit <= 0 {
		limit = 20
	}

	if rows, err = JobRuns.SelectAll(); err != nil {
		return
	}

	runs = dereference(rows)
	slices.SortFunc(runs, func(a JobRun, b JobRun) (cmp int) {
		cmp = b.StartTime.Compare(a.StartTime)
		return
	})

	if len(runs) > limit {
		runs = runs[:limit]
	}

	return
}

// ListJobActionRunsByRunIDs returns action runs for the selected job runs.
func (r *Repository) ListJobActionRunsByRunIDs(ctx context.Context, runIDs []int) (runs []JobActionRun, err error) {
	var (
		rows   []*JobActionRun
		filter *gosqlite.Filter
		ids    []uint64
	)

	if err = ctx.Err(); err != nil {
		return
	}

	ids = make([]uint64, 0, len(runIDs))
	for _, runID := range runIDs {
		if runID > 0 {
			ids = append(ids, uint64(runID))
		}
	}

	if len(ids) == 0 {
		runs = []JobActionRun{}
		return
	}

	filter = gosqlite.NewFilter().KeyCmp(JobActionRuns.FieldByGoName("JobRunID"), gosqlite.OpIn, ids)
	if rows, err = JobActionRuns.SelectAllWithFilter(filter); err != nil {
		return
	}

	runs = dereference(rows)
	slices.SortFunc(runs, func(a JobActionRun, b JobActionRun) (cmp int) {
		cmp = b.StartTime.Compare(a.StartTime)
		return
	})

	return
}

// ListJobHostResultsByRunIDs returns host results for the selected job runs.
func (r *Repository) ListJobHostResultsByRunIDs(ctx context.Context, runIDs []int) (results []JobHostResult, err error) {
	var (
		rows   []*JobHostResult
		filter *gosqlite.Filter
		ids    []uint64
	)

	if err = ctx.Err(); err != nil {
		return
	}

	ids = make([]uint64, 0, len(runIDs))
	for _, runID := range runIDs {
		if runID > 0 {
			ids = append(ids, uint64(runID))
		}
	}

	if len(ids) == 0 {
		results = []JobHostResult{}
		return
	}

	filter = gosqlite.NewFilter().KeyCmp(JobHostResults.FieldByGoName("JobRunID"), gosqlite.OpIn, ids)
	if rows, err = JobHostResults.SelectAllWithFilter(filter); err != nil {
		return
	}

	results = dereference(rows)
	slices.SortFunc(results, func(a JobHostResult, b JobHostResult) (cmp int) {
		cmp = strings.Compare(strings.ToLower(a.Hostname), strings.ToLower(b.Hostname))
		return
	})

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

// GetJobRun returns a single job run by primary key.
func (r *Repository) GetJobRun(ctx context.Context, id int) (run *JobRun, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if id <= 0 {
		err = fmt.Errorf("job run id must be positive")
		return
	}

	if run, err = JobRuns.Select(id); err != nil {
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

// InsertJobRun persists a new job run.
func (r *Repository) InsertJobRun(ctx context.Context, run *JobRun) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if err = JobRuns.Insert(run); err != nil {
		return
	}

	return
}

// UpdateJobRun persists an existing job run.
func (r *Repository) UpdateJobRun(ctx context.Context, run *JobRun) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if err = JobRuns.Update(run); err != nil {
		return
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

// UpdateJobAction persists an existing job action.
func (r *Repository) UpdateJobAction(ctx context.Context, action *JobAction) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if err = JobActions.Update(action); err != nil {
		return
	}

	return
}

// InsertJobActionRun persists a new job action run.
func (r *Repository) InsertJobActionRun(ctx context.Context, run *JobActionRun) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if err = JobActionRuns.Insert(run); err != nil {
		return
	}

	return
}

// UpdateJobActionRun persists an existing job action run.
func (r *Repository) UpdateJobActionRun(ctx context.Context, run *JobActionRun) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if err = JobActionRuns.Update(run); err != nil {
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

// DeleteJobAction removes one job action by primary key.
func (r *Repository) DeleteJobAction(ctx context.Context, actionID int) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if actionID <= 0 {
		err = fmt.Errorf("job action id must be positive")
		return
	}

	if _, err = JobActions.DeleteWithFilter(
		gosqlite.NewFilter().KeyCmp(JobActions.FieldByGoName("ID"), gosqlite.OpEqual, uint64(actionID)),
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
		gosqlite.NewFilter().KeyCmp(JobActions.FieldByGoName("JobID"), gosqlite.OpEqual, uint64(jobID)),
	); err != nil {
		return
	}

	actions = dereference(rows)
	slices.SortFunc(actions, func(a JobAction, b JobAction) (cmp int) {
		cmp = a.Position - b.Position
		return
	})

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
