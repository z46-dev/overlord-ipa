package handlers

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/overlord-ipa/db"
)

type IPAService interface {
	SyncHosts(ctx context.Context) error
	GetHosts(ctx context.Context) ([]db.Host, error)
	GetHostGroups(ctx context.Context) ([]string, error)
}

type HostsHandler struct {
	ipa IPAService
}

// NewHostsHandler creates host API handlers.
func NewHostsHandler(ipa IPAService) (handler *HostsHandler) {
	handler = &HostsHandler{ipa: ipa}
	return
}

// List writes the host inventory response.
func (h *HostsHandler) List(c fiber.Ctx) (err error) {
	var hosts []db.Host
	if hosts, err = h.ipa.GetHosts(c.Context()); err != nil {
		err = writeError(c, err)
		return
	}

	if err = c.JSON(hosts); err != nil {
		return
	}

	return
}

// Groups writes the FreeIPA host group response.
func (h *HostsHandler) Groups(c fiber.Ctx) (err error) {
	var groups []string
	if groups, err = h.ipa.GetHostGroups(c.Context()); err != nil {
		err = writeError(c, err)
		return
	}

	if err = c.JSON(groups); err != nil {
		return
	}

	return
}
