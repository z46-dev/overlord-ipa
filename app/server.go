package app

import (
	"fmt"
	"net"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/overlord-ipa/app/handlers"
	"github.com/z46-dev/overlord-ipa/conf"
)

type Server struct {
	app *fiber.App
}

type ServerDependencies struct {
	Auth      *handlers.AuthHandler
	Data      *handlers.DataHandler
	Dashboard *handlers.DashboardHandler
	Hosts     *handlers.HostsHandler
	Jobs      *handlers.JobsHandler
}

// NewServer creates the HTTP server and registers routes.
func NewServer(app *fiber.App, dependencies ServerDependencies) (server *Server) {
	server = &Server{app: app}
	server.RegisterRoutes(dependencies)
	return server
}

// Listen starts the main server and any configured redirect listeners.
func (s *Server) Listen() (err error) {
	var (
		serverConf conf.ServerConfig = conf.Conf.Server
		tlsEnabled bool
		errs       chan error
	)

	if tlsEnabled, err = serverTLSConfigured(serverConf.TLSCertFile, serverConf.TLSKeyFile); err != nil {
		return
	}

	errs = make(chan error, len(serverConf.RedirectPorts)+1)
	for _, redirectPort := range serverConf.RedirectPorts {
		if redirectPort < 1 || redirectPort > 65535 {
			err = fmt.Errorf("invalid redirect port %d: port must be between 1 and 65535", redirectPort)
			return
		}

		var redirectApp *fiber.App = newRedirectApp(serverConf.Host, serverConf.Port, tlsEnabled)
		go func() {
			var addr string = net.JoinHostPort(serverConf.Host, fmt.Sprint(redirectPort))
			errs <- redirectApp.Listen(addr, fiber.ListenConfig{
				DisableStartupMessage: true,
			})
		}()
	}

	go func() {
		var (
			addr         string             = net.JoinHostPort(serverConf.Host, fmt.Sprint(serverConf.Port))
			listenConfig fiber.ListenConfig = fiber.ListenConfig{}
		)

		if tlsEnabled {
			listenConfig.CertFile = serverConf.TLSCertFile
			listenConfig.CertKeyFile = serverConf.TLSKeyFile
		}

		errs <- s.app.Listen(addr, listenConfig)
	}()

	err = <-errs
	return
}

// serverTLSConfigured validates TLS file pairing and reports whether TLS is enabled.
func serverTLSConfigured(certFile string, keyFile string) (enabled bool, err error) {
	switch {
	case certFile == "" && keyFile == "":
		enabled = false
	case certFile != "" && keyFile != "":
		enabled = true
	default:
		err = fmt.Errorf("tls_cert_file and tls_key_file must both be set to enable HTTPS")
	}

	return
}

// newRedirectApp creates a listener that redirects to the canonical server URL.
func newRedirectApp(host string, port uint, tlsEnabled bool) (redirectApp *fiber.App) {
	var (
		scheme     string = "http"
		targetBase string
	)

	if tlsEnabled {
		scheme = "https"
	}

	targetBase = fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, fmt.Sprint(port)))

	redirectApp = fiber.New(fiber.Config{
		AppName: "Overlord IPA Redirect",
	})

	redirectApp.All("/*", func(c fiber.Ctx) (err error) {
		var requestURI string = c.OriginalURL()
		if requestURI == "" || !strings.HasPrefix(requestURI, "/") {
			requestURI = "/" + requestURI
		}

		if err = c.Redirect().Status(fiber.StatusPermanentRedirect).To(targetBase + requestURI); err != nil {
			return
		}

		return
	})

	return redirectApp
}
