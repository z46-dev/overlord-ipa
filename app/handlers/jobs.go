package handlers

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/overlord-ipa/app/services"
	"github.com/z46-dev/overlord-ipa/db"
)

type JobService interface {
	ListJobs(ctx context.Context) ([]db.Job, error)
	ListJobActions(ctx context.Context, jobID int) ([]db.JobAction, error)
	CreateJob(ctx context.Context, input services.JobInput) (*db.Job, error)
	UpdateJob(ctx context.Context, jobID int, input services.JobInput) (*db.Job, error)
	RunJob(ctx context.Context, jobID int, triggeredBy string) (*db.JobRun, error)
}

type SchedulerService interface {
	Snapshot() services.SchedulerSnapshot
}

type JobsHandler struct {
	jobs      JobService
	scheduler SchedulerService
}

// NewJobsHandler creates job API handlers.
func NewJobsHandler(jobs JobService, scheduler SchedulerService) (handler *JobsHandler) {
	handler = &JobsHandler{
		jobs:      jobs,
		scheduler: scheduler,
	}
	return
}

// List writes configured jobs and scheduler state.
func (h *JobsHandler) List(c fiber.Ctx) (err error) {
	var (
		jobs    []db.Job
		actions map[int][]db.JobAction
	)

	if jobs, err = h.jobs.ListJobs(c.Context()); err != nil {
		err = writeError(c, err)
		return
	}

	actions = make(map[int][]db.JobAction, len(jobs))
	for _, job := range jobs {
		if actions[job.ID], err = h.jobs.ListJobActions(c.Context(), job.ID); err != nil {
			err = writeError(c, err)
			return
		}
	}

	if err = c.JSON(fiber.Map{
		"jobs":      jobs,
		"actions":   actions,
		"scheduler": h.scheduler.Snapshot(),
	}); err != nil {
		return
	}

	return
}

// Create writes a new job from editor input.
func (h *JobsHandler) Create(c fiber.Ctx) (err error) {
	var (
		input services.JobInput
		job   *db.Job
	)

	if err = c.Bind().Body(&input); err != nil {
		err = writeError(c, services.NewInvalidInputError("invalid job request", err))
		return
	}

	if job, err = h.jobs.CreateJob(c.Context(), input); err != nil {
		err = writeError(c, err)
		return
	}

	if err = c.Status(fiber.StatusCreated).JSON(job); err != nil {
		return
	}

	return
}

// Update writes changes to an existing job.
func (h *JobsHandler) Update(c fiber.Ctx) (err error) {
	var (
		jobID int
		input services.JobInput
		job   *db.Job
	)

	if jobID, err = strconv.Atoi(c.Params("id")); err != nil || jobID <= 0 {
		err = writeError(c, services.NewInvalidInputError("job id must be a positive integer", err))
		return
	}

	if err = c.Bind().Body(&input); err != nil {
		err = writeError(c, services.NewInvalidInputError("invalid job request", err))
		return
	}

	if job, err = h.jobs.UpdateJob(c.Context(), jobID, input); err != nil {
		err = writeError(c, err)
		return
	}

	if err = c.JSON(job); err != nil {
		return
	}

	return
}

// Run starts a job execution for editors.
func (h *JobsHandler) Run(c fiber.Ctx) (err error) {
	var (
		jobID       int
		triggeredBy string = "api"
		user        *services.AuthenticatedUser
		run         *db.JobRun
	)

	if jobID, err = strconv.Atoi(c.Params("id")); err != nil || jobID <= 0 {
		err = writeError(c, services.NewInvalidInputError("job id must be a positive integer", err))
		return
	}

	user = CurrentUser(c)
	if user != nil {
		triggeredBy = user.Username
	}

	if run, err = h.jobs.RunJob(c.Context(), jobID, triggeredBy); err != nil {
		err = writeError(c, err)
		return
	}

	if err = c.Status(fiber.StatusAccepted).JSON(run); err != nil {
		return
	}

	return
}
