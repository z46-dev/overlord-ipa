package handlers

import (
	"context"
	"strings"
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
	Username string `json:"username" form:"username"`
	Password string `json:"password" form:"password"`
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
		if isFormLogin(c) {
			err = redirectLoginError(c)
			return
		}

		err = writeError(c, services.NewInvalidInputError("invalid login request", err))
		return
	}

	if session, err = h.service.Login(c.Context(), request.Username, request.Password); err != nil {
		if isFormLogin(c) {
			err = redirectLoginError(c)
			return
		}

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

	if isFormLogin(c) {
		if err = c.Redirect().Status(fiber.StatusSeeOther).To("/"); err != nil {
			return
		}

		return
	}

	if err = c.JSON(session.User); err != nil {
		return
	}

	return
}

// LoginPage serves the browser-native login page used by password managers.
func (h *AuthHandler) LoginPage(c fiber.Ctx) (err error) {
	var token string = c.Cookies(h.service.CookieName())
	if token != "" {
		if _, err = h.service.GetSession(c.Context(), token); err == nil {
			if err = c.Redirect().Status(fiber.StatusSeeOther).To("/"); err != nil {
				return
			}

			return
		}
	}

	c.Set(fiber.HeaderContentType, "text/html; charset=utf-8")
	if err = c.SendString(loginPageHTML(c.Query("login_error") != "")); err != nil {
		return
	}

	return
}

func isFormLogin(c fiber.Ctx) bool {
	var contentType string = strings.ToLower(string(c.Request().Header.ContentType()))
	return strings.HasPrefix(contentType, "application/x-www-form-urlencoded") || strings.HasPrefix(contentType, "multipart/form-data")
}

func redirectLoginError(c fiber.Ctx) (err error) {
	if err = c.Redirect().Status(fiber.StatusSeeOther).To("/login?login_error=1"); err != nil {
		return
	}

	return
}

func loginPageHTML(showError bool) string {
	var errorHTML string
	if showError {
		errorHTML = `<div class="error">Login failed</div>`
	}

	return `<!doctype html>
<html lang="en">
    <head>
        <meta charset="UTF-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1.0" />
        <title>Overlord IPA Login</title>
        <style>
            :root {
                color: #1f2933;
                background: #f5f5f5;
                font-family: "Public Sans", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
            }
            * {
                box-sizing: border-box;
            }
            body {
                margin: 0;
                min-height: 100vh;
                background: #f5f5f5;
            }
            header {
                height: 48px;
                display: flex;
                align-items: center;
                gap: 24px;
                padding: 0 20px;
                border-bottom: 1px solid #1f2327;
                background: #2b2f33;
                color: #fff;
            }
            .brand {
                font-size: 14px;
                font-weight: 600;
            }
            .subhead {
                color: #d1d5db;
                font-size: 12px;
            }
            main {
                width: min(100% - 32px, 448px);
                margin: 64px auto 0;
                border: 1px solid #d1d5db;
                border-radius: 4px;
                background: #fff;
            }
            h1 {
                margin: 0;
                padding: 12px 16px;
                border-bottom: 1px solid #d1d5db;
                font-size: 16px;
                line-height: 24px;
            }
            form {
                display: grid;
                gap: 16px;
                padding: 16px;
            }
            label {
                display: grid;
                gap: 4px;
                font-size: 14px;
                font-weight: 500;
            }
            input {
                width: 100%;
                border: 1px solid #d1d5db;
                border-radius: 2px;
                padding: 8px 12px;
                font: inherit;
                font-size: 14px;
                outline: none;
            }
            input:focus {
                border-color: #1f6fb2;
            }
            button {
                width: 100%;
                border: 0;
                border-radius: 2px;
                background: #1f6fb2;
                color: #fff;
                padding: 9px 12px;
                font: inherit;
                font-size: 14px;
                font-weight: 600;
                cursor: pointer;
            }
            button:hover {
                background: #155a96;
            }
            .error {
                border: 1px solid #fca5a5;
                border-radius: 4px;
                background: #fef2f2;
                color: #991b1b;
                padding: 8px 12px;
                font-size: 14px;
            }
        </style>
    </head>
    <body>
        <header>
            <div class="brand">Overlord IPA</div>
            <div class="subhead">Infrastructure automation</div>
        </header>
        <main>
            <h1>Log in with FreeIPA</h1>
            <form action="/api/auth/login" method="post" autocomplete="on">
                ` + errorHTML + `
                <label for="username">
                    Username
                    <input id="username" name="username" type="text" autocomplete="username" autocapitalize="none" spellcheck="false" required />
                </label>
                <label for="password">
                    Password
                    <input id="password" name="password" type="password" autocomplete="current-password" required />
                </label>
                <button type="submit">Sign in</button>
            </form>
        </main>
    </body>
</html>`
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
