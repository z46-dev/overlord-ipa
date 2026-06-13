package app

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v3"
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
}

// New wires application dependencies.
func New() (application *Application, err error) {
	var (
		repository       *db.Repository           = db.NewRepository()
		freeIPA          *services.FreeIPAService = services.NewFreeIPAService(conf.Conf)
		authService      *services.AuthService
		dataFileService  *services.DataFileService
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
	jobService = services.NewJobService(repository, services.NewDefaultActionFactory(), freeIPA, dataFileService)
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

	if err = a.server.Listen(); err != nil {
		return
	}

	return
}

// Init creates and runs the application.
func Init() (err error) {
	var application *Application
	if application, err = New(); err != nil {
		return
	}

	if err = application.Run(context.Background()); err != nil {
		return
	}

	return
}
