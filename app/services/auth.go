package services

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/z46-dev/overlord-ipa/conf"
)

type Role string

const (
	RoleViewer Role = "viewer"
	RoleEditor Role = "editor"
)

type AuthenticatedUser struct {
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	DN          string    `json:"dn"`
	Groups      []string  `json:"groups"`
	Roles       []Role    `json:"roles"`
	CanView     bool      `json:"can_view"`
	CanEdit     bool      `json:"can_edit"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Session struct {
	Token     string             `json:"-"`
	User      *AuthenticatedUser `json:"user"`
	ExpiresAt time.Time          `json:"expires_at"`
}

type AuthService struct {
	config       conf.Config
	viewerGroups map[string]struct{}
	editorGroups map[string]struct{}
	sessionTTL   time.Duration
	sessions     map[string]*Session
	mu           sync.RWMutex
}

// NewAuthService creates the LDAP-backed authentication service.
func NewAuthService(config conf.Config) (service *AuthService, err error) {
	var sessionTTL time.Duration
	if sessionTTL, err = time.ParseDuration(config.Auth.SessionTTL); err != nil {
		err = fmt.Errorf("parse auth session ttl: %w", err)
		return
	}

	if sessionTTL <= 0 {
		err = fmt.Errorf("auth session ttl must be positive")
		return
	}

	service = &AuthService{
		config:       config,
		viewerGroups: buildGroupSet(config.Auth.ViewerGroups),
		editorGroups: buildGroupSet(config.Auth.EditorGroups),
		sessionTTL:   sessionTTL,
		sessions:     make(map[string]*Session),
	}
	return
}

// CookieName returns the configured session cookie name.
func (s *AuthService) CookieName() (name string) {
	name = s.config.Auth.SessionCookie
	return
}

// Login authenticates LDAP credentials and creates an in-memory session.
func (s *AuthService) Login(ctx context.Context, username string, password string) (session *Session, err error) {
	var (
		user      *AuthenticatedUser
		token     string
		expiresAt time.Time
	)

	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		err = NewInvalidInputError("username and password are required", nil)
		return
	}

	if err = ctx.Err(); err != nil {
		return
	}

	if user, err = s.authenticateLDAP(ctx, username, password); err != nil {
		err = NewUnauthorizedError("invalid username, password, or group membership", err)
		return
	}

	if token, err = newSessionToken(); err != nil {
		err = NewExecutionError("create session token", err)
		return
	}

	expiresAt = time.Now().UTC().Add(s.sessionTTL)
	user.ExpiresAt = expiresAt
	session = &Session{
		Token:     token,
		User:      user,
		ExpiresAt: expiresAt,
	}

	s.mu.Lock()
	s.sessions[token] = session
	s.pruneExpiredLocked(time.Now().UTC())
	s.mu.Unlock()

	return
}

// GetSession resolves and validates an existing session token.
func (s *AuthService) GetSession(ctx context.Context, token string) (session *Session, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	if token == "" {
		err = NewUnauthorizedError("login required", nil)
		return
	}

	var (
		now time.Time = time.Now().UTC()
		ok  bool
	)

	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok = s.sessions[token]; !ok {
		err = NewUnauthorizedError("invalid session", nil)
		return
	}

	if !session.ExpiresAt.After(now) {
		delete(s.sessions, token)
		err = NewUnauthorizedError("session expired", nil)
		return
	}

	session = cloneSession(session)
	return
}

// Logout deletes a session token.
func (s *AuthService) Logout(ctx context.Context, token string) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
	return
}

// authenticateLDAP validates credentials and group authorization.
func (s *AuthService) authenticateLDAP(ctx context.Context, username string, password string) (user *AuthenticatedUser, err error) {
	var (
		opsConn   *ldap.Conn
		userEntry *ldap.Entry
		groups    []string
		roles     []Role
	)

	if opsConn, err = s.connect(); err != nil {
		return
	}
	defer opsConn.Close()

	if err = opsConn.Bind(s.config.FreeIPA.BindDN, s.config.FreeIPA.BindPassword); err != nil {
		err = fmt.Errorf("bind operation user: %w", err)
		return
	}

	if userEntry, err = s.findUser(ctx, opsConn, username); err != nil {
		return
	}

	if err = s.bindAsUser(userEntry.DN, password); err != nil {
		return
	}

	if groups, err = s.findUserGroups(ctx, opsConn, username, userEntry); err != nil {
		return
	}

	roles = s.rolesForGroups(groups)
	if len(roles) == 0 {
		err = fmt.Errorf("user %s is not in an allowed viewer or editor group", username)
		return
	}

	user = &AuthenticatedUser{
		Username:    username,
		DisplayName: firstNonEmpty(userEntry.GetAttributeValue("displayName"), userEntry.GetAttributeValue("cn"), username),
		DN:          userEntry.DN,
		Groups:      groups,
		Roles:       roles,
		CanView:     true,
		CanEdit:     containsRole(roles, RoleEditor),
	}

	return
}

// connect creates a configured LDAP connection.
func (s *AuthService) connect() (conn *ldap.Conn, err error) {
	var (
		tlsConfig      *tls.Config
		connectTimeout time.Duration
		requestTimeout time.Duration
		scheme         string = "ldap"
		addr           string
	)

	if tlsConfig, err = s.tlsConfig(); err != nil {
		return
	}

	if connectTimeout, err = time.ParseDuration(s.config.FreeIPA.ConnectTimeout); err != nil {
		err = fmt.Errorf("parse ldap connect timeout: %w", err)
		return
	}

	if requestTimeout, err = time.ParseDuration(s.config.FreeIPA.RequestTimeout); err != nil {
		err = fmt.Errorf("parse ldap request timeout: %w", err)
		return
	}

	if s.config.FreeIPA.UseTLS && !s.config.FreeIPA.StartTLS {
		scheme = "ldaps"
	}

	addr = fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(s.config.FreeIPA.Server, fmt.Sprint(s.config.FreeIPA.Port)))
	if conn, err = ldap.DialURL(
		addr,
		ldap.DialWithDialer(&net.Dialer{Timeout: connectTimeout}),
		ldap.DialWithTLSConfig(tlsConfig),
	); err != nil {
		err = fmt.Errorf("connect ldap: %w", err)
		return
	}

	conn.SetTimeout(requestTimeout)

	if s.config.FreeIPA.StartTLS {
		if err = conn.StartTLS(tlsConfig); err != nil {
			conn.Close()
			err = fmt.Errorf("start tls: %w", err)
			return
		}
	}

	return
}

// tlsConfig builds TLS settings for LDAP connections.
func (s *AuthService) tlsConfig() (config *tls.Config, err error) {
	var (
		certPEM []byte
		pool    *x509.CertPool
		ok      bool
	)

	config = &tls.Config{
		ServerName:         s.config.FreeIPA.Server,
		InsecureSkipVerify: s.config.FreeIPA.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	if s.config.FreeIPA.CACertFile == "" {
		return
	}

	if certPEM, err = os.ReadFile(s.config.FreeIPA.CACertFile); err != nil {
		err = fmt.Errorf("read ldap ca cert: %w", err)
		return
	}

	if pool, err = x509.SystemCertPool(); err != nil {
		pool = x509.NewCertPool()
	}

	if ok = pool.AppendCertsFromPEM(certPEM); !ok {
		err = fmt.Errorf("parse ldap ca cert")
		return
	}

	config.RootCAs = pool
	return
}

// findUser searches LDAP for a username.
func (s *AuthService) findUser(ctx context.Context, conn *ldap.Conn, username string) (entry *ldap.Entry, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	var (
		request *ldap.SearchRequest
		result  *ldap.SearchResult
	)

	request = ldap.NewSearchRequest(
		s.config.FreeIPA.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		1,
		0,
		false,
		fmt.Sprintf("(&(objectClass=person)(uid=%s))", ldap.EscapeFilter(username)),
		[]string{"dn", "uid", "cn", "displayName", "memberOf"},
		nil,
	)

	if result, err = conn.Search(request); err != nil {
		err = fmt.Errorf("search ldap user: %w", err)
		return
	}

	if len(result.Entries) != 1 {
		err = fmt.Errorf("ldap user not found")
		return
	}

	entry = result.Entries[0]
	return
}

// bindAsUser validates a user's password by binding as that user.
func (s *AuthService) bindAsUser(userDN string, password string) (err error) {
	var conn *ldap.Conn
	if conn, err = s.connect(); err != nil {
		return
	}
	defer conn.Close()

	if err = conn.Bind(userDN, password); err != nil {
		err = fmt.Errorf("bind user: %w", err)
		return
	}

	return
}

// findUserGroups returns the LDAP groups associated with a user.
func (s *AuthService) findUserGroups(ctx context.Context, conn *ldap.Conn, username string, userEntry *ldap.Entry) (groups []string, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	var (
		filter string = fmt.Sprintf(
			"(&(|(objectClass=groupOfNames)(objectClass=posixGroup)(objectClass=groupOfUniqueNames))(|(member=%s)(memberUid=%s)(uniqueMember=%s)))",
			ldap.EscapeFilter(userEntry.DN),
			ldap.EscapeFilter(username),
			ldap.EscapeFilter(userEntry.DN),
		)
		request *ldap.SearchRequest
		result  *ldap.SearchResult
		seen    map[string]struct{}
	)

	request = ldap.NewSearchRequest(
		s.config.FreeIPA.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		filter,
		[]string{"dn", "cn"},
		nil,
	)

	if result, err = conn.Search(request); err != nil {
		err = fmt.Errorf("search ldap user groups: %w", err)
		return
	}

	groups = make([]string, 0, len(result.Entries)*2+len(userEntry.GetAttributeValues("memberOf")))
	seen = make(map[string]struct{}, len(result.Entries)*2)

	for _, groupDN := range userEntry.GetAttributeValues("memberOf") {
		addUniqueGroup(&groups, seen, groupDN)
	}

	for _, entry := range result.Entries {
		addUniqueGroup(&groups, seen, entry.DN)
		addUniqueGroup(&groups, seen, entry.GetAttributeValue("cn"))
	}

	return
}

// rolesForGroups maps LDAP groups to application roles.
func (s *AuthService) rolesForGroups(groups []string) (roles []Role) {
	var (
		hasViewer  bool
		hasEditor  bool
		normalized string
		ok         bool
	)

	for _, group := range groups {
		normalized = normalizeGroup(group)
		if _, ok = s.viewerGroups[normalized]; ok {
			hasViewer = true
		}

		if _, ok = s.editorGroups[normalized]; ok {
			hasEditor = true
		}
	}

	roles = make([]Role, 0, 2)
	if hasViewer || hasEditor {
		roles = append(roles, RoleViewer)
	}

	if hasEditor {
		roles = append(roles, RoleEditor)
	}

	return
}

// pruneExpiredLocked deletes expired sessions while the mutex is held.
func (s *AuthService) pruneExpiredLocked(now time.Time) {
	for token, session := range s.sessions {
		if !session.ExpiresAt.After(now) {
			delete(s.sessions, token)
		}
	}
}

// newSessionToken creates a random session token.
func newSessionToken() (token string, err error) {
	var bytes []byte = make([]byte, 32)
	if _, err = rand.Read(bytes); err != nil {
		return
	}

	token = hex.EncodeToString(bytes)
	return
}

// buildGroupSet normalizes configured group names for lookup.
func buildGroupSet(groups []string) (set map[string]struct{}) {
	var normalized string

	set = make(map[string]struct{}, len(groups))
	for _, group := range groups {
		normalized = normalizeGroup(group)
		if normalized != "" {
			set[normalized] = struct{}{}
		}
	}

	return
}

// normalizeGroup returns a case-insensitive group lookup key.
func normalizeGroup(group string) (normalized string) {
	normalized = strings.ToLower(strings.TrimSpace(group))
	return
}

// addUniqueGroup appends a group once to a group list.
func addUniqueGroup(groups *[]string, seen map[string]struct{}, group string) {
	var (
		normalized string = normalizeGroup(group)
		ok         bool
	)

	if normalized == "" {
		return
	}

	if _, ok = seen[normalized]; ok {
		return
	}

	seen[normalized] = struct{}{}
	*groups = append(*groups, group)
}

// containsRole reports whether a role is present.
func containsRole(roles []Role, role Role) (ok bool) {
	for _, candidate := range roles {
		if candidate == role {
			ok = true
			return
		}
	}

	return
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) (selected string) {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			selected = value
			return
		}
	}

	return
}

// cloneSession copies a session before returning it to callers.
func cloneSession(session *Session) (cloned *Session) {
	var clonedUser AuthenticatedUser = *session.User
	clonedUser.Groups = append([]string(nil), session.User.Groups...)
	clonedUser.Roles = append([]Role(nil), session.User.Roles...)

	cloned = &Session{
		Token:     session.Token,
		User:      &clonedUser,
		ExpiresAt: session.ExpiresAt,
	}
	return
}
