package handlers

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/overlord-ipa/app/services"
)

type currentUserKey struct{}

type AuthService interface {
	Login(ctx context.Context, username string, password string) (*services.Session, error)
	GetSession(ctx context.Context, token string) (*services.Session, error)
	Logout(ctx context.Context, token string) error
	CookieName() string
}

type AuthHandler struct {
	service AuthService
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// NewAuthHandler creates an HTTP auth handler.
func NewAuthHandler(service AuthService) (handler *AuthHandler) {
	handler = &AuthHandler{service: service}
	return
}

// Login authenticates a user and creates a session cookie.
func (h *AuthHandler) Login(c fiber.Ctx) (err error) {
	var (
		request loginRequest
		session *services.Session
	)

	if err = c.Bind().Body(&request); err != nil {
		err = writeError(c, services.NewInvalidInputError("invalid login request", err))
		return
	}

	if session, err = h.service.Login(c.Context(), request.Username, request.Password); err != nil {
		err = writeError(c, err)
		return
	}

	c.Cookie(&fiber.Cookie{
		Name:     h.service.CookieName(),
		Value:    session.Token,
		Path:     "/",
		Expires:  session.ExpiresAt,
		MaxAge:   int(time.Until(session.ExpiresAt).Seconds()),
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
	})

	if err = c.JSON(session.User); err != nil {
		return
	}

	return
}

// Me returns the currently authenticated user.
func (h *AuthHandler) Me(c fiber.Ctx) (err error) {
	var session *services.Session
	if session, err = h.sessionFromRequest(c); err != nil {
		err = writeError(c, err)
		return
	}

	if err = c.JSON(session.User); err != nil {
		return
	}

	return
}

// Logout clears the current session.
func (h *AuthHandler) Logout(c fiber.Ctx) (err error) {
	var token string = c.Cookies(h.service.CookieName())
	if token != "" {
		if err = h.service.Logout(c.Context(), token); err != nil {
			err = writeError(c, err)
			return
		}
	}

	c.ClearCookie(h.service.CookieName())
	if err = c.SendStatus(fiber.StatusNoContent); err != nil {
		return
	}

	return
}

// RequireViewer enforces an authenticated viewer session.
func (h *AuthHandler) RequireViewer(c fiber.Ctx) (err error) {
	var session *services.Session
	if session, err = h.sessionFromRequest(c); err != nil {
		err = writeError(c, err)
		return
	}

	c.Locals(currentUserKey{}, session.User)
	if err = c.Next(); err != nil {
		return
	}

	return
}

// RequireEditor enforces an authenticated editor session.
func (h *AuthHandler) RequireEditor(c fiber.Ctx) (err error) {
	var session *services.Session
	if session, err = h.sessionFromRequest(c); err != nil {
		err = writeError(c, err)
		return
	}

	if !session.User.CanEdit {
		err = writeError(c, services.NewForbiddenError("editor access is required", nil))
		return
	}

	c.Locals(currentUserKey{}, session.User)
	if err = c.Next(); err != nil {
		return
	}

	return
}

// sessionFromRequest resolves a session from the request cookie.
func (h *AuthHandler) sessionFromRequest(c fiber.Ctx) (session *services.Session, err error) {
	var token string = c.Cookies(h.service.CookieName())
	if token == "" {
		err = services.NewUnauthorizedError("login required", nil)
		return
	}

	if session, err = h.service.GetSession(c.Context(), token); err != nil {
		return
	}

	return
}

// CurrentUser returns the request user stored by auth middleware.
func CurrentUser(c fiber.Ctx) (user *services.AuthenticatedUser) {
	var ok bool
	if user, ok = c.Locals(currentUserKey{}).(*services.AuthenticatedUser); !ok {
		user = nil
	}

	return
}
