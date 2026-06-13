package handlers

import (
	"context"
	"path/filepath"

	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/overlord-ipa/app/services"
)

type DataFileService interface {
	ListFiles(ctx context.Context) ([]services.DataFileInfo, error)
	ReadFile(ctx context.Context, path string) (services.DataFileContent, error)
	WriteFile(ctx context.Context, input services.DataFileInput) (services.DataFileContent, error)
	DeleteFile(ctx context.Context, path string) error
}

type DataHandler struct {
	files DataFileService
}

type dataFilePathRequest struct {
	Path string `json:"path"`
}

// NewDataHandler creates data file API handlers.
func NewDataHandler(files DataFileService) (handler *DataHandler) {
	handler = &DataHandler{files: files}
	return
}

// List writes data directory file metadata.
func (h *DataHandler) List(c fiber.Ctx) (err error) {
	var files []services.DataFileInfo
	if files, err = h.files.ListFiles(c.Context()); err != nil {
		err = writeError(c, err)
		return
	}

	if err = c.JSON(files); err != nil {
		return
	}

	return
}

// Read writes a single data file with content.
func (h *DataHandler) Read(c fiber.Ctx) (err error) {
	var file services.DataFileContent
	if file, err = h.files.ReadFile(c.Context(), c.Query("path")); err != nil {
		err = writeError(c, err)
		return
	}

	if err = c.JSON(file); err != nil {
		return
	}

	return
}

// Save writes an editable data file.
func (h *DataHandler) Save(c fiber.Ctx) (err error) {
	var (
		input services.DataFileInput
		file  services.DataFileContent
	)

	if err = c.Bind().Body(&input); err != nil {
		err = writeError(c, services.NewInvalidInputError("invalid data file request", err))
		return
	}

	if file, err = h.files.WriteFile(c.Context(), input); err != nil {
		err = writeError(c, err)
		return
	}

	if err = c.JSON(file); err != nil {
		return
	}

	return
}

// Delete removes an editable data file.
func (h *DataHandler) Delete(c fiber.Ctx) (err error) {
	var request dataFilePathRequest
	if err = c.Bind().Body(&request); err != nil {
		err = writeError(c, services.NewInvalidInputError("invalid data file request", err))
		return
	}

	if err = h.files.DeleteFile(c.Context(), request.Path); err != nil {
		err = writeError(c, err)
		return
	}

	if err = c.SendStatus(fiber.StatusNoContent); err != nil {
		return
	}

	return
}

// Download streams a data file as an attachment.
func (h *DataHandler) Download(c fiber.Ctx) (err error) {
	var file services.DataFileContent
	if file, err = h.files.ReadFile(c.Context(), c.Query("path")); err != nil {
		err = writeError(c, err)
		return
	}

	c.Set(fiber.HeaderContentDisposition, "attachment; filename="+filepath.Base(file.Path))
	c.Set(fiber.HeaderContentType, "application/octet-stream")
	if err = c.SendString(file.Content); err != nil {
		return
	}

	return
}
