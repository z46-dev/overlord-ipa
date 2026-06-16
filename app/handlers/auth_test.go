package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/overlord-ipa/app/services"
)

type mockAuthService struct {
	loginSession *services.Session
	loginErr     error
	username     string
	password     string
}

func (s *mockAuthService) Login(ctx context.Context, username string, password string) (*services.Session, error) {
	s.username = username
	s.password = password
	return s.loginSession, s.loginErr
}

func (s *mockAuthService) GetSession(ctx context.Context, token string) (*services.Session, error) {
	return nil, nil
}

func (s *mockAuthService) Logout(ctx context.Context, token string) error {
	return nil
}

func (s *mockAuthService) CookieName() string {
	return "test_session"
}

func TestLoginFormRedirectsAndSetsCookie(t *testing.T) {
	authService := &mockAuthService{loginSession: testSession()}
	response := testLoginRequest(t, authService, "application/x-www-form-urlencoded", strings.NewReader(url.Values{
		"username": {"alice"},
		"password": {"secret"},
	}.Encode()))
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", fiber.StatusSeeOther, response.StatusCode)
	}
	if location := response.Header.Get("Location"); location != "/" {
		t.Fatalf("expected redirect to /, got %q", location)
	}
	if authService.username != "alice" || authService.password != "secret" {
		t.Fatalf("unexpected credentials: username=%q password=%q", authService.username, authService.password)
	}

	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Name != "test_session" || cookies[0].Value != "token-123" {
		t.Fatalf("expected test_session cookie, got %#v", cookies)
	}
}

func TestLoginJSONStillReturnsUser(t *testing.T) {
	authService := &mockAuthService{loginSession: testSession()}
	response := testLoginRequest(t, authService, "application/json", strings.NewReader(`{"username":"alice","password":"secret"}`))
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status %d, got %d", fiber.StatusOK, response.StatusCode)
	}

	var user services.AuthenticatedUser
	if err := json.NewDecoder(response.Body).Decode(&user); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if user.Username != "alice" {
		t.Fatalf("expected JSON user alice, got %q", user.Username)
	}
}

func TestLoginFormFailureRedirectsError(t *testing.T) {
	authService := &mockAuthService{loginErr: services.NewUnauthorizedError("invalid username or password", nil)}
	response := testLoginRequest(t, authService, "application/x-www-form-urlencoded", strings.NewReader(url.Values{
		"username": {"alice"},
		"password": {"bad"},
	}.Encode()))
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", fiber.StatusSeeOther, response.StatusCode)
	}
	if location := response.Header.Get("Location"); location != "/login?login_error=1" {
		t.Fatalf("expected redirect to login error, got %q", location)
	}
}

func TestLoginPageIncludesNativeCredentialForm(t *testing.T) {
	authService := &mockAuthService{}

	app := fiber.New()
	handler := NewAuthHandler(authService)
	app.Get("/login", handler.LoginPage)

	request, err := http.NewRequest(http.MethodGet, "/login?login_error=1", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("run request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status %d, got %d", fiber.StatusOK, response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	for _, expected := range []string{
		`<form action="/api/auth/login" method="post" autocomplete="on">`,
		`name="username"`,
		`autocomplete="username"`,
		`name="password"`,
		`autocomplete="current-password"`,
		`Login failed`,
	} {
		if !strings.Contains(string(body), expected) {
			t.Fatalf("expected login page to contain %q", expected)
		}
	}
}

func testLoginRequest(t *testing.T, authService *mockAuthService, contentType string, body *strings.Reader) *http.Response {
	t.Helper()

	app := fiber.New()
	handler := NewAuthHandler(authService)
	app.Post("/api/auth/login", handler.Login)

	request, err := http.NewRequest(http.MethodPost, "/api/auth/login", body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Content-Type", contentType)

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("run request: %v", err)
	}

	return response
}

func testSession() *services.Session {
	expiresAt := time.Now().UTC().Add(time.Hour)
	return &services.Session{
		Token:     "token-123",
		ExpiresAt: expiresAt,
		User: &services.AuthenticatedUser{
			Username:  "alice",
			CanView:   true,
			ExpiresAt: expiresAt,
		},
	}
}
