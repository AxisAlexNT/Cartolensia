package auth

import (
	"errors"
	"net/http"
	"time"
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

type Session struct {
	ID        string    `json:"id"`
	Principal Principal `json:"principal"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Authenticator interface {
	Authenticate(*http.Request) (Principal, error)
}

type Authorizer interface {
	Authorize(Principal, string) error
}

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
