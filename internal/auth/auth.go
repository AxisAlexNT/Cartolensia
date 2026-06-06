package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/AxisAlexNT/Cartolensia/internal/id"
)

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrUnauthorized    = errors.New("unauthorized")
)

type Principal struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Email      string   `json:"email,omitempty"`
	Role       string   `json:"role"`
	AuthMethod string   `json:"auth_method,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
}

type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	DisplayName  string     `json:"display_name"`
	PasswordHash string     `json:"-"`
	Role         string     `json:"role"`
	DisabledAt   *time.Time `json:"disabled_at,omitempty"`
}

type Session struct {
	ID        string    `json:"id"`
	Principal Principal `json:"principal"`
	ExpiresAt time.Time `json:"expires_at"`
}

type APIToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

type TokenResult struct {
	Token  APIToken `json:"token"`
	Secret string   `json:"secret"`
}

type LoginResult struct {
	Principal Principal `json:"principal"`
	Session   Session   `json:"session"`
}

type Authenticator interface {
	Authenticate(*http.Request) (Principal, error)
}

type Authorizer interface {
	Authorize(Principal, string) error
}

type Store interface {
	BootstrapAdmin(context.Context, User) (User, bool, error)
	UserByEmail(context.Context, string) (User, error)
	UserByID(context.Context, string) (User, error)
	UpdatePassword(context.Context, string, string) error
	CreateSession(context.Context, string, string, []byte, time.Time) error
	PrincipalBySessionHash(context.Context, []byte, time.Time) (Principal, error)
	DeleteSessionByHash(context.Context, []byte) error
	DeleteExpiredSessions(context.Context, time.Time) (int64, error)
	PrincipalByAPITokenHash(context.Context, []byte, time.Time) (Principal, error)
	CreateAPIToken(context.Context, APIToken, []byte) error
	ListAPITokens(context.Context, string) ([]APIToken, error)
	RevokeAPIToken(context.Context, string, string) error
}

type Config struct {
	AdminEmail              string
	AdminDisplayName        string
	AdminPasswordEnv        string
	RotateBootstrapPassword bool
	SessionTTL              time.Duration
	APITokenTTL             time.Duration
	CookieName              string
	CookieSecure            bool
	CSRFHeader              string
}

const DefaultCookieName = "cartolensia_session"

const (
	AuthMethodDevNoAuth = "dev_no_auth"
	AuthMethodSession   = "session"
	AuthMethodAPIToken  = "api_token"

	ScopeRead         = "read"
	ScopeWrite        = "write"
	ScopeJobsWrite    = "jobs:write"
	ScopePluginsWrite = "plugins:write"
	ScopeMediaRead    = "media:read"
	ScopeAdmin        = "admin"

	DefaultCSRFHeader = "X-CSRF-Token"
)

type DevNoAuth struct{}

func (DevNoAuth) Authenticate(*http.Request) (Principal, error) {
	return Principal{ID: "dev", Name: "Development Admin", Role: "admin", AuthMethod: AuthMethodDevNoAuth, Scopes: []string{ScopeAdmin}}, nil
}

func (DevNoAuth) Authorize(principal Principal, _ string) error {
	if principal.ID == "" {
		return ErrUnauthenticated
	}
	return nil
}

type DisabledLocalAuth struct{}

func (DisabledLocalAuth) Authenticate(*http.Request) (Principal, error) {
	return Principal{}, ErrUnauthenticated
}

func (DisabledLocalAuth) Authorize(principal Principal, _ string) error {
	if principal.ID == "" {
		return ErrUnauthenticated
	}
	if principal.Role != "admin" {
		return ErrUnauthorized
	}
	return nil
}

type LocalService struct {
	store Store
	cfg   Config
}

func NewLocalService(store Store, cfg Config) *LocalService {
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 24 * time.Hour
	}
	if cfg.APITokenTTL <= 0 {
		cfg.APITokenTTL = 90 * 24 * time.Hour
	}
	if cfg.CookieName == "" {
		cfg.CookieName = DefaultCookieName
	}
	if cfg.CSRFHeader == "" {
		cfg.CSRFHeader = DefaultCSRFHeader
	}
	if cfg.AdminPasswordEnv == "" {
		cfg.AdminPasswordEnv = "CARTOLENSIA_ADMIN_PASSWORD"
	}
	return &LocalService{store: store, cfg: cfg}
}

func (s *LocalService) Bootstrap(ctx context.Context, password string) (User, bool, error) {
	email := strings.ToLower(strings.TrimSpace(s.cfg.AdminEmail))
	if email == "" {
		return User{}, false, errors.New("auth.admin_email is required for local auth bootstrap")
	}
	if password == "" {
		if existing, err := s.store.UserByEmail(ctx, email); err == nil {
			return existing, false, nil
		}
		return User{}, false, fmt.Errorf("%s must be set for local auth bootstrap", s.cfg.AdminPasswordEnv)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, false, err
	}
	name := strings.TrimSpace(s.cfg.AdminDisplayName)
	if name == "" {
		name = "Cartolensia Admin"
	}
	user, created, err := s.store.BootstrapAdmin(ctx, User{
		ID:           id.NewUUID(),
		Email:        email,
		DisplayName:  name,
		PasswordHash: hash,
		Role:         "admin",
	})
	if err != nil || created || !s.cfg.RotateBootstrapPassword {
		return user, created, err
	}
	if err := s.store.UpdatePassword(ctx, user.ID, hash); err != nil {
		return User{}, false, err
	}
	user.PasswordHash = hash
	return user, false, nil
}

func (s *LocalService) Authenticate(r *http.Request) (Principal, error) {
	credential, ok := s.CredentialFromRequest(r)
	if !ok {
		return Principal{}, ErrUnauthenticated
	}
	tokenHash := TokenHash(credential.Secret)
	now := time.Now().UTC()
	_, _ = s.store.DeleteExpiredSessions(r.Context(), now)
	if credential.Method == AuthMethodSession {
		principal, err := s.store.PrincipalBySessionHash(r.Context(), tokenHash, now)
		if err == nil {
			principal.AuthMethod = AuthMethodSession
			return principal, nil
		}
		return Principal{}, err
	}
	if principal, err := s.store.PrincipalByAPITokenHash(r.Context(), tokenHash, now); err == nil {
		principal.AuthMethod = AuthMethodAPIToken
		return principal, nil
	}
	return Principal{}, ErrUnauthenticated
}

func (s *LocalService) Authorize(principal Principal, action string) error {
	if principal.ID == "" {
		return ErrUnauthenticated
	}
	if principal.AuthMethod != AuthMethodAPIToken && principal.Role == "admin" {
		return nil
	}
	if principal.AuthMethod == AuthMethodAPIToken && scopeAllowed(principal.Scopes, action) {
		return nil
	}
	if principal.Role != "admin" {
		return ErrUnauthorized
	}
	return ErrUnauthorized
}

func (s *LocalService) Login(ctx context.Context, email, password string) (LoginResult, string, error) {
	_, _ = s.store.DeleteExpiredSessions(ctx, time.Now().UTC())
	user, err := s.store.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return LoginResult{}, "", ErrUnauthenticated
	}
	if user.DisabledAt != nil || !CheckPassword(user.PasswordHash, password) {
		return LoginResult{}, "", ErrUnauthenticated
	}
	secret, err := RandomSecret(32)
	if err != nil {
		return LoginResult{}, "", err
	}
	expiresAt := time.Now().UTC().Add(s.cfg.SessionTTL)
	sessionID := id.NewUUID()
	if err := s.store.CreateSession(ctx, sessionID, user.ID, TokenHash(secret), expiresAt); err != nil {
		return LoginResult{}, "", err
	}
	principal := Principal{ID: user.ID, Name: user.DisplayName, Email: user.Email, Role: user.Role, AuthMethod: AuthMethodSession}
	return LoginResult{Principal: principal, Session: Session{ID: sessionID, Principal: principal, ExpiresAt: expiresAt}}, secret, nil
}

func (s *LocalService) ChangePassword(ctx context.Context, principal Principal, oldPassword, newPassword string) error {
	if principal.ID == "" {
		return ErrUnauthenticated
	}
	if strings.TrimSpace(newPassword) == "" {
		return errors.New("new password is required")
	}
	user, err := s.store.UserByID(ctx, principal.ID)
	if err != nil {
		return ErrUnauthenticated
	}
	if !CheckPassword(user.PasswordHash, oldPassword) {
		return ErrUnauthenticated
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.store.UpdatePassword(ctx, principal.ID, hash)
}

func (s *LocalService) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.DeleteSessionByHash(ctx, TokenHash(token))
}

func (s *LocalService) CreateAPIToken(ctx context.Context, principal Principal, name string, scopes []string, expiresAt *time.Time) (TokenResult, error) {
	if err := s.Authorize(principal, "api_tokens.create"); err != nil {
		return TokenResult{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return TokenResult{}, errors.New("token name is required")
	}
	secret, err := RandomSecret(32)
	if err != nil {
		return TokenResult{}, err
	}
	if expiresAt == nil && s.cfg.APITokenTTL > 0 {
		expires := time.Now().UTC().Add(s.cfg.APITokenTTL)
		expiresAt = &expires
	}
	token := APIToken{ID: id.NewUUID(), UserID: principal.ID, Name: name, Scopes: cleanScopes(scopes), ExpiresAt: expiresAt, CreatedAt: time.Now().UTC()}
	if err := s.store.CreateAPIToken(ctx, token, TokenHash(secret)); err != nil {
		return TokenResult{}, err
	}
	return TokenResult{Token: token, Secret: secret}, nil
}

func (s *LocalService) ListAPITokens(ctx context.Context, principal Principal) ([]APIToken, error) {
	if err := s.Authorize(principal, "api_tokens.list"); err != nil {
		return nil, err
	}
	return s.store.ListAPITokens(ctx, principal.ID)
}

func (s *LocalService) RevokeAPIToken(ctx context.Context, principal Principal, tokenID string) error {
	if err := s.Authorize(principal, "api_tokens.revoke"); err != nil {
		return err
	}
	return s.store.RevokeAPIToken(ctx, principal.ID, tokenID)
}

func (s *LocalService) CookieName() string { return s.cfg.CookieName }

func (s *LocalService) CookieSecure() bool { return s.cfg.CookieSecure }

func (s *LocalService) CSRFHeader() string { return s.cfg.CSRFHeader }

type Credential struct {
	Secret string
	Method string
}

func (s *LocalService) CredentialFromRequest(r *http.Request) (Credential, bool) {
	if value := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(strings.ToLower(value), "bearer ") {
		token := strings.TrimSpace(value[len("bearer "):])
		return Credential{Secret: token, Method: AuthMethodAPIToken}, token != ""
	}
	cookie, err := r.Cookie(s.cfg.CookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return Credential{}, false
	}
	return Credential{Secret: cookie.Value, Method: AuthMethodSession}, true
}

func (s *LocalService) TokenFromRequest(r *http.Request) (string, bool) {
	credential, ok := s.CredentialFromRequest(r)
	return credential.Secret, ok
}

func (s *LocalService) CSRFToken(secret string) string {
	sum := sha256.Sum256([]byte("cartolensia-csrf-v1:" + secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *LocalService) ValidateCSRF(r *http.Request) error {
	credential, ok := s.CredentialFromRequest(r)
	if !ok || credential.Method != AuthMethodSession {
		return nil
	}
	expected := s.CSRFToken(credential.Secret)
	actual := strings.TrimSpace(r.Header.Get(s.cfg.CSRFHeader))
	if actual == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		return ErrUnauthorized
	}
	return nil
}

func HashPassword(password string) (string, error) {
	data, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func CheckPassword(hash, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func TokenHash(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

func RandomSecret(bytes int) (string, error) {
	if bytes <= 0 {
		bytes = 32
	}
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func ConstantTimeTokenEqual(left, right []byte) bool {
	return subtle.ConstantTimeCompare(left, right) == 1
}

func cleanScopes(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}

func scopeAllowed(scopes []string, action string) bool {
	needed := scopesForAction(action)
	available := map[string]struct{}{}
	for _, scope := range scopes {
		available[scope] = struct{}{}
	}
	if _, ok := available[ScopeAdmin]; ok {
		return true
	}
	for _, scope := range needed {
		if _, ok := available[scope]; ok {
			return true
		}
	}
	return false
}

func scopesForAction(action string) []string {
	switch action {
	case "jobs.cancel", "jobs.retry", "discovery.start", "hash.start", "metadata.enrich", "previews.generate":
		return []string{ScopeJobsWrite, ScopeWrite}
	case "plugins.rescan":
		return []string{ScopePluginsWrite, ScopeWrite}
	case "api_tokens.create", "api_tokens.list", "api_tokens.revoke", "auth.password.change":
		return []string{ScopeAdmin}
	case "sync.links.save", "sync.links.delete":
		return []string{ScopeWrite}
	default:
		return []string{ScopeWrite}
	}
}
