package handlers

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/overlord-ipa/app/services"
)

type DashboardService interface {
	Summary(ctx context.Context) (services.DashboardSummary, error)
}

type DashboardHandler struct {
	service DashboardService
}

// NewDashboardHandler creates dashboard API handlers.
func NewDashboardHandler(service DashboardService) (handler *DashboardHandler) {
	handler = &DashboardHandler{service: service}
	return
}

// Summary writes the dashboard summary response.
func (h *DashboardHandler) Summary(c fiber.Ctx) (err error) {
	var summary services.DashboardSummary
	if summary, err = h.service.Summary(c.Context()); err != nil {
		err = writeError(c, err)
		return
	}

	if err = c.JSON(summary); err != nil {
		return
	}

	return
}

type errorResponse struct {
	Error serviceErrorResponse `json:"error"`
}

type serviceErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError converts service errors into API error responses.
func writeError(c fiber.Ctx, err error) (writeErr error) {
	var (
		status   int                  = fiber.StatusInternalServerError
		response serviceErrorResponse = serviceErrorResponse{
			Code:    "internal_error",
			Message: "internal server error",
		}
		serviceErr *services.ServiceError
	)

	if errors.As(err, &serviceErr) {
		response.Code = serviceErr.Code
		response.Message = serviceErr.Message
		switch serviceErr.Code {
		case services.ErrorCodeInvalidInput:
			status = fiber.StatusBadRequest
		case services.ErrorCodeNotFound:
			status = fiber.StatusNotFound
		case services.ErrorCodeUnauthorized:
			status = fiber.StatusUnauthorized
		case services.ErrorCodeForbidden:
			status = fiber.StatusForbidden
		}
	}

	if writeErr = c.Status(status).JSON(errorResponse{Error: response}); writeErr != nil {
		return
	}

	return
}
