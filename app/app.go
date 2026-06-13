package app

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/golog"
	"github.com/z46-dev/overlord-ipa/app/handlers"
	"github.com/z46-dev/overlord-ipa/app/services"
	"github.com/z46-dev/overlord-ipa/conf"
	"github.com/z46-dev/overlord-ipa/db"
)

type Application struct {
	server    *Server
	scheduler *services.Scheduler
	data      *services.DataFileService
	jobs      *services.JobService
	queue     *services.JobQueue
}

// New wires application dependencies.
func New(logger *golog.Logger) (application *Application, err error) {
	var (
		repository       *db.Repository           = db.NewRepository()
		freeIPA          *services.FreeIPAService = services.NewFreeIPAService(conf.Conf, repository, logger)
		authService      *services.AuthService
		dataFileService  *services.DataFileService
		jobQueue         *services.JobQueue
		ansibleRunner    *services.GoAnsibleRunner
		inventoryWriter  *services.AnsibleInventoryWriter
		jobService       *services.JobService
		scheduler        *services.Scheduler
		dashboardService *services.DashboardService
		fiberApp         *fiber.App
		server           *Server
	)

	if authService, err = services.NewAuthService(conf.Conf); err != nil {
		return
	}

	dataFileService = services.NewDataFileService(conf.Conf.Data)
	if jobQueue, err = services.NewJobQueue(conf.Conf.Gasket, logger); err != nil {
		return
	}

	ansibleRunner = services.NewGoAnsibleRunner(conf.Conf.Ansible, dataFileService, logger)
	inventoryWriter = services.NewAnsibleInventoryWriter(conf.Conf.Ansible, logger)
	jobService = services.NewJobService(repository, services.NewDefaultActionFactory(), freeIPA, dataFileService, jobQueue, ansibleRunner, inventoryWriter, logger)
	if err = jobQueue.RegisterJobRunConsumer(func(payload services.JobRunTaskPayload) (data []byte, err error) {
		data, err = jobService.ExecuteQueuedJob(context.Background(), payload)
		return
	}); err != nil {
		return
	}

	scheduler = services.NewScheduler(repository, services.NewMockExecutor())
	dashboardService = services.NewDashboardService(repository)

	fiberApp = fiber.New(fiber.Config{
		AppName: "Overlord IPA",
	})

	server = NewServer(fiberApp, ServerDependencies{
		Auth:      handlers.NewAuthHandler(authService),
		Data:      handlers.NewDataHandler(dataFileService),
		Dashboard: handlers.NewDashboardHandler(dashboardService),
		Hosts:     handlers.NewHostsHandler(freeIPA),
		Jobs:      handlers.NewJobsHandler(jobService, scheduler),
	})

	application = &Application{
		server:    server,
		scheduler: scheduler,
		data:      dataFileService,
		jobs:      jobService,
		queue:     jobQueue,
	}
	return
}

// Run loads schedulable jobs and starts the web server.
func (a *Application) Run(ctx context.Context) (err error) {
	if err = a.data.EnsureDefaultFiles(ctx); err != nil {
		err = fmt.Errorf("seed default files: %w", err)
		return
	}

	if err = a.jobs.EnsureDefaultJobs(ctx); err != nil {
		err = fmt.Errorf("seed default jobs: %w", err)
		return
	}

	if err = a.scheduler.Load(ctx); err != nil {
		err = fmt.Errorf("load scheduler: %w", err)
		return
	}

	go func() {
		var queueErr error
		if queueErr = a.queue.Run(ctx); queueErr != nil && ctx.Err() == nil {
			fmt.Printf("job queue stopped: %v\n", queueErr)
		}
	}()

	if err = a.server.Listen(); err != nil {
		return
	}

	return
}

// Init creates and runs the application.
func Init(logger *golog.Logger) (err error) {
	var application *Application
	if application, err = New(logger); err != nil {
		return
	}

	if err = application.Run(context.Background()); err != nil {
		return
	}

	return
}
