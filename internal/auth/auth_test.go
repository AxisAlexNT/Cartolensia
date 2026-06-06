package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLocalServiceBootstrapLoginLogoutAndToken(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	service := NewLocalService(store, Config{
		AdminEmail:       "Admin@Example.Local",
		AdminDisplayName: "Admin",
		AdminPasswordEnv: "TEST_ADMIN_PASSWORD",
		SessionTTL:       time.Hour,
		APITokenTTL:      time.Hour,
		CookieName:       "test_session",
	})
	user, created, err := service.Bootstrap(ctx, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !created || user.Email != "admin@example.local" {
		t.Fatalf("unexpected bootstrap: user=%#v created=%v", user, created)
	}
	if _, _, err := service.Login(ctx, user.Email, "wrong"); err != ErrUnauthenticated {
		t.Fatalf("expected unauthenticated login, got %v", err)
	}
	result, sessionSecret, err := service.Login(ctx, user.Email, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if result.Principal.ID != user.ID || sessionSecret == "" {
		t.Fatalf("unexpected login result: %#v secret=%q", result, sessionSecret)
	}
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: service.CookieName(), Value: sessionSecret})
	principal, err := service.Authenticate(req)
	if err != nil {
		t.Fatal(err)
	}
	if principal.Email != user.Email {
		t.Fatalf("unexpected principal: %#v", principal)
	}
	token, err := service.CreateAPIToken(ctx, principal, "smoke", []string{"jobs:write"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bearerReq := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	bearerReq.Header.Set("Authorization", "Bearer "+token.Secret)
	tokenPrincipal, err := service.Authenticate(bearerReq)
	if err != nil {
		t.Fatal(err)
	}
	if tokenPrincipal.ID != user.ID {
		t.Fatalf("unexpected token principal: %#v", tokenPrincipal)
	}
	if err := service.Authorize(tokenPrincipal, "plugins.rescan"); err != ErrUnauthorized {
		t.Fatalf("expected plugin scope denial, got %v", err)
	}
	if err := service.Authorize(tokenPrincipal, "jobs.cancel"); err != nil {
		t.Fatalf("expected jobs scope to allow cancel: %v", err)
	}
	if err := service.ChangePassword(ctx, principal, "wrong", "new password"); err != ErrUnauthenticated {
		t.Fatalf("expected password change denial, got %v", err)
	}
	if err := service.ChangePassword(ctx, principal, "correct horse battery staple", "new password"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Login(ctx, user.Email, "correct horse battery staple"); err != ErrUnauthenticated {
		t.Fatalf("old password still worked: %v", err)
	}
	if _, _, err := service.Login(ctx, user.Email, "new password"); err != nil {
		t.Fatalf("new password failed: %v", err)
	}
	if err := service.Logout(ctx, sessionSecret); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(req); err != ErrUnauthenticated {
		t.Fatalf("expected logged out session, got %v", err)
	}
}

func TestLocalServiceRejectsExpiredSessions(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	service := NewLocalService(store, Config{
		AdminEmail:       "admin@example.local",
		AdminDisplayName: "Admin",
		SessionTTL:       time.Hour,
		APITokenTTL:      time.Hour,
		CookieName:       "test_session",
	})
	user, _, err := service.Bootstrap(ctx, "password")
	if err != nil {
		t.Fatal(err)
	}
	secret := "expired-session"
	if err := store.CreateSession(ctx, "session-1", user.ID, TokenHash(secret), time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: service.CookieName(), Value: secret})
	if _, err := service.Authenticate(req); err != ErrUnauthenticated {
		t.Fatalf("expected expired session rejection, got %v", err)
	}
	deleted, err := store.DeleteExpiredSessions(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatalf("expired session should already be cleaned during auth, deleted=%d", deleted)
	}
}
