package app

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/z46-dev/overlord-ipa/conf"
)

// RegisterRoutes attaches API and static frontend routes.
func (s *Server) RegisterRoutes(dependencies ServerDependencies) {
	var api fiber.Router = s.app.Group("/api")

	api.Get("/health", func(c fiber.Ctx) (err error) {
		if err = c.JSON(fiber.Map{
			"status": "ok",
			"time":   time.Now().UTC(),
		}); err != nil {
			return
		}

		return
	})

	api.Post("/auth/login", dependencies.Auth.Login)
	api.Get("/auth/me", dependencies.Auth.Me)
	api.Post("/auth/logout", dependencies.Auth.Logout)

	s.app.Get("/login", dependencies.Auth.LoginPage)

	var viewerAPI fiber.Router = api.Group("", dependencies.Auth.RequireViewer)
	viewerAPI.Get("/dashboard/summary", dependencies.Dashboard.Summary)
	viewerAPI.Get("/hosts", dependencies.Hosts.List)
	viewerAPI.Get("/hostgroups", dependencies.Hosts.Groups)
	viewerAPI.Get("/jobs", dependencies.Jobs.List)

	var editorAPI fiber.Router = api.Group("", dependencies.Auth.RequireEditor)
	editorAPI.Post("/jobs", dependencies.Jobs.Create)
	editorAPI.Put("/jobs/:id", dependencies.Jobs.Update)
	editorAPI.Post("/jobs/:id/run", dependencies.Jobs.Run)
	editorAPI.Get("/data/files", dependencies.Data.List)
	editorAPI.Get("/data/file", dependencies.Data.Read)
	editorAPI.Get("/data/download", dependencies.Data.Download)
	editorAPI.Post("/data/file", dependencies.Data.Save)
	editorAPI.Put("/data/file", dependencies.Data.Save)
	editorAPI.Delete("/data/file", dependencies.Data.Delete)

	s.registerStaticRoutes()
}

// registerStaticRoutes serves the built React app and optional SPA fallback.
func (s *Server) registerStaticRoutes() {
	var (
		staticDir    string = conf.Conf.Server.StaticDir
		indexFile    string = filepath.Join(staticDir, "index.html")
		staticConfig static.Config
	)

	staticConfig = static.Config{
		Next: func(c fiber.Ctx) bool {
			var skip bool = strings.HasPrefix(c.Path(), "/api")
			return skip
		},
		NotFoundHandler: func(c fiber.Ctx) (err error) {
			var method string

			if !conf.Conf.Server.SPAFallback {
				if err = c.Next(); err != nil {
					return
				}

				return
			}

			method = c.Method()
			if method != fiber.MethodGet && method != fiber.MethodHead {
				if err = c.Next(); err != nil {
					return
				}

				return
			}

			if err = c.Status(fiber.StatusOK).SendFile(indexFile); err != nil {
				return
			}

			return
		},
	}

	s.app.Use("/", static.New(staticDir, staticConfig))
}
