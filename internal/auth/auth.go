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
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Role  string `json:"role"`
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
	CreateSession(context.Context, string, string, []byte, time.Time) error
	PrincipalBySessionHash(context.Context, []byte, time.Time) (Principal, error)
	DeleteSessionByHash(context.Context, []byte) error
	PrincipalByAPITokenHash(context.Context, []byte, time.Time) (Principal, error)
	CreateAPIToken(context.Context, APIToken, []byte) error
	ListAPITokens(context.Context, string) ([]APIToken, error)
	RevokeAPIToken(context.Context, string, string) error
}

type Config struct {
	AdminEmail       string
	AdminDisplayName string
	AdminPasswordEnv string
	SessionTTL       time.Duration
	APITokenTTL      time.Duration
	CookieName       string
}

const DefaultCookieName = "cartolensia_session"

type DevNoAuth struct{}

func (DevNoAuth) Authenticate(*http.Request) (Principal, error) {
	return Principal{ID: "dev", Name: "Development Admin", Role: "admin"}, nil
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
	if cfg.AdminPasswordEnv == "" {
		cfg.AdminPasswordEnv = "CARTOLENSIA_ADMIN_PASSWORD"
	}
	return &LocalService{store: store, cfg: cfg}
}

func (s *LocalService) Bootstrap(ctx context.Context, password string) (User, bool, error) {
	if strings.TrimSpace(s.cfg.AdminEmail) == "" {
		return User{}, false, errors.New("auth.admin_email is required for local auth bootstrap")
	}
	if password == "" {
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
	return s.store.BootstrapAdmin(ctx, User{
		ID:           id.NewUUID(),
		Email:        strings.ToLower(strings.TrimSpace(s.cfg.AdminEmail)),
		DisplayName:  name,
		PasswordHash: hash,
		Role:         "admin",
	})
}

func (s *LocalService) Authenticate(r *http.Request) (Principal, error) {
	token, ok := s.TokenFromRequest(r)
	if !ok {
		return Principal{}, ErrUnauthenticated
	}
	tokenHash := TokenHash(token)
	now := time.Now().UTC()
	if principal, err := s.store.PrincipalBySessionHash(r.Context(), tokenHash, now); err == nil {
		return principal, nil
	}
	return s.store.PrincipalByAPITokenHash(r.Context(), tokenHash, now)
}

func (s *LocalService) Authorize(principal Principal, _ string) error {
	if principal.ID == "" {
		return ErrUnauthenticated
	}
	if principal.Role != "admin" {
		return ErrUnauthorized
	}
	return nil
}

func (s *LocalService) Login(ctx context.Context, email, password string) (LoginResult, string, error) {
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
	principal := Principal{ID: user.ID, Name: user.DisplayName, Email: user.Email, Role: user.Role}
	return LoginResult{Principal: principal, Session: Session{ID: sessionID, Principal: principal, ExpiresAt: expiresAt}}, secret, nil
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

func (s *LocalService) TokenFromRequest(r *http.Request) (string, bool) {
	if value := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(strings.ToLower(value), "bearer ") {
		token := strings.TrimSpace(value[len("bearer "):])
		return token, token != ""
	}
	cookie, err := r.Cookie(s.cfg.CookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}
	return cookie.Value, true
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
