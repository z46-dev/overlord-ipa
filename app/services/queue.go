package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/z46-dev/gasket"
	"github.com/z46-dev/golog"
	"github.com/z46-dev/overlord-ipa/conf"
)

const GasketTaskTypeJobRun string = "job.run"

type JobRunTaskPayload struct {
	JobID       int      `json:"job_id"`
	JobRunID    int      `json:"job_run_id"`
	TriggeredBy string   `json:"triggered_by"`
	HostGroups  []string `json:"host_groups"`
}

type JobRunTaskConsumer func(payload JobRunTaskPayload) (data []byte, err error)

type JobQueue struct {
	client     *gasket.Client
	retryCount int
	retryDelay time.Duration
	log        *golog.Logger
}

// NewJobQueue creates the embedded Gasket-backed job queue.
func NewJobQueue(config conf.GasketConfig, logger *golog.Logger) (queue *JobQueue, err error) {
	var (
		pollInterval   time.Duration
		lockRetryDelay time.Duration
		recovery       time.Duration
		retryDelay     time.Duration
		client         *gasket.Client
		databaseDir    string
		log            *golog.Logger = serviceLogger(logger, "[QUEUE]", golog.BoldCyan)
	)

	if pollInterval, err = parseDurationWithDefault(config.PollInterval, 250*time.Millisecond); err != nil {
		err = NewInvalidInputError("invalid gasket poll interval", err)
		return
	}

	if lockRetryDelay, err = parseDurationWithDefault(config.LockRetryDelay, 5*time.Millisecond); err != nil {
		err = NewInvalidInputError("invalid gasket lock retry delay", err)
		return
	}

	if recovery, err = parseDurationWithDefault(config.TaskRecoveryTimeout, 5*time.Minute); err != nil {
		err = NewInvalidInputError("invalid gasket recovery timeout", err)
		return
	}

	if retryDelay, err = parseDurationWithDefault(config.RetryDelay, 30*time.Second); err != nil {
		err = NewInvalidInputError("invalid gasket retry delay", err)
		return
	}

	databaseDir = filepath.Dir(strings.TrimSpace(config.DatabaseFile))
	if config.DatabaseFile != ":memory:" && databaseDir != "." && databaseDir != "" {
		if err = os.MkdirAll(databaseDir, 0750); err != nil {
			err = NewExecutionError("create gasket database directory", err)
			return
		}
	}

	if client, err = gasket.NewClient(
		config.DatabaseFile,
		gasket.PollInterval(pollInterval),
		gasket.DatabaseLockRetry(config.LockRetryCount, lockRetryDelay),
		gasket.TaskRecoveryTimeout(recovery),
	); err != nil {
		err = NewExecutionError("open gasket queue", err)
		return
	}

	queue = &JobQueue{
		client:     client,
		retryCount: config.RetryCount,
		retryDelay: retryDelay,
		log:        log,
	}

	if queue.log != nil {
		queue.log.Infof("Opened Gasket queue database=%s retry_count=%d retry_delay=%s\n", config.DatabaseFile, config.RetryCount, retryDelay)
	}

	return
}

// EnqueueJobRun schedules a job-run task for worker execution.
func (q *JobQueue) EnqueueJobRun(ctx context.Context, payload JobRunTaskPayload) (taskID int, err error) {
	var (
		data []byte
		info *gasket.TaskInfo
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if q.log != nil {
		q.log.Infof("Enqueueing job run job_id=%d run_id=%d triggered_by=%s groups=%v\n", payload.JobID, payload.JobRunID, payload.TriggeredBy, payload.HostGroups)
	}

	if data, err = json.Marshal(payload); err != nil {
		err = NewInvalidInputError("encode job task payload", err)
		return
	}

	if info, err = q.client.NewTask(GasketTaskTypeJobRun, data, gasket.RetryPolicy(q.retryCount, q.retryDelay)); err != nil {
		err = NewExecutionError("enqueue job run", err)
		return
	}

	taskID = info.ID()
	if q.log != nil {
		q.log.Infof("Queued job run job_id=%d run_id=%d task_id=%d\n", payload.JobID, payload.JobRunID, taskID)
	}

	return
}

// RegisterJobRunConsumer binds the job-run consumer used by queue workers.
func (q *JobQueue) RegisterJobRunConsumer(consumer JobRunTaskConsumer) (err error) {
	if consumer == nil {
		err = NewInvalidInputError("job queue consumer is required", nil)
		return
	}

	if q.log != nil {
		q.log.Info("Registering job run queue consumer\n")
	}

	if err = q.client.RegisterConsumer(GasketTaskTypeJobRun, func(id int, payload []byte) (result gasket.TaskConsumerResult) {
		var (
			jobPayload JobRunTaskPayload
			data       []byte
			runErr     error
		)

		if runErr = json.Unmarshal(payload, &jobPayload); runErr != nil {
			if q.log != nil {
				q.log.Errorf("Failed to decode task_id=%d payload: %v\n", id, runErr)
			}

			result = gasket.TaskConsumerResult{
				Success: false,
				Error:   runErr,
			}
			return
		}

		if data, runErr = consumer(jobPayload); runErr != nil {
			if q.log != nil {
				q.log.Errorf("Job run task failed task_id=%d job_id=%d run_id=%d: %v\n", id, jobPayload.JobID, jobPayload.JobRunID, runErr)
			}

			if isNonRetryableJobFailure(runErr) {
				result = gasket.TaskConsumerResult{
					Success: true,
					Data:    []byte(runErr.Error()),
				}
				return
			}

			result = gasket.TaskConsumerResult{
				Success: false,
				Error:   runErr,
			}
			return
		}

		result = gasket.TaskConsumerResult{
			Success: true,
			Data:    data,
		}
		return
	}); err != nil {
		err = NewExecutionError("register job queue consumer", err)
		return
	}

	return
}

// Run starts processing queued tasks until the context is canceled.
func (q *JobQueue) Run(ctx context.Context) (err error) {
	if q.log != nil {
		q.log.Info("Starting Gasket queue worker\n")
	}

	if err = q.client.Run(ctx); err != nil {
		err = NewExecutionError("run gasket queue", err)
		return
	}

	if q.log != nil {
		q.log.Info("Stopped Gasket queue worker\n")
	}

	return
}

// Close releases queue resources.
func (q *JobQueue) Close() (err error) {
	if err = q.client.Close(); err != nil {
		err = NewExecutionError("close gasket queue", err)
		return
	}

	return
}

// isNonRetryableJobFailure reports completed job failures that should not be requeued.
func isNonRetryableJobFailure(err error) (nonRetryable bool) {
	var serviceErr *ServiceError

	if errors.As(err, &serviceErr) {
		nonRetryable = serviceErr.Code == ErrorCodeExecution || serviceErr.Code == ErrorCodeInvalidInput
	}

	return
}

// parseDurationWithDefault parses a configured duration with a fallback.
func parseDurationWithDefault(value string, fallback time.Duration) (duration time.Duration, err error) {
	if value == "" {
		duration = fallback
		return
	}

	if duration, err = time.ParseDuration(value); err != nil {
		return
	}

	return
}
