package httperr

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

// Test sentinel errors — registered in init() to avoid importing domain
// packages (which would create a cycle since they now import httperr).
var (
	errSentinel404 = errors.New("test: not found")
	errSentinel400 = errors.New("test: bad request")
	errSentinel409 = errors.New("test: conflict")
	errSentinel401 = errors.New("test: unauthorized")
	errSentinel500 = errors.New("test: internal error")
)

func init() {
	Register(errSentinel404, http.StatusNotFound, "test: not found")
	Register(errSentinel400, http.StatusBadRequest, "test: bad request")
	Register(errSentinel409, http.StatusConflict, "test: conflict")
	Register(errSentinel401, http.StatusUnauthorized, "test: unauthorized")
	Register(errSentinel500, http.StatusInternalServerError, "internal server error")
}

func TestMapHTTP_KnownError(t *testing.T) {
	status, msg := MapHTTP(errSentinel404)
	if status != http.StatusNotFound {
		t.Errorf("got status %d, want %d", status, http.StatusNotFound)
	}
	if msg != "test: not found" {
		t.Errorf("got message %q, want %q", msg, "test: not found")
	}
}

func TestMapHTTP_AllRegisteredStatuses(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "404", err: errSentinel404, status: http.StatusNotFound},
		{name: "400", err: errSentinel400, status: http.StatusBadRequest},
		{name: "409", err: errSentinel409, status: http.StatusConflict},
		{name: "401", err: errSentinel401, status: http.StatusUnauthorized},
		{name: "500", err: errSentinel500, status: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg := MapHTTP(tt.err)
			if status != tt.status {
				t.Errorf("got status %d, want %d", status, tt.status)
			}
			if msg == "" {
				t.Error("message must not be empty")
			}
		})
	}
}

func TestMapHTTP_UnknownErrorDefaultsTo500(t *testing.T) {
	status, msg := MapHTTP(errors.New("some unknown error"))
	if status != http.StatusInternalServerError {
		t.Errorf("got status %d, want %d", status, http.StatusInternalServerError)
	}
	if msg != "internal server error" {
		t.Errorf("got message %q, want %q", msg, "internal server error")
	}
}

func TestMapHTTP_WrappedError(t *testing.T) {
	wrapped := fmt.Errorf("wrap: %w", errSentinel404)
	status, _ := MapHTTP(wrapped)
	if status != http.StatusNotFound {
		t.Errorf("got status %d, want %d", status, http.StatusNotFound)
	}
}

func TestRegister_PanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when registering nil error")
		}
	}()
	Register(nil, 200, "ok")
}

func TestRegister_Overwrite(t *testing.T) {
	// Registering the same error again should overwrite.
	Register(errSentinel400, http.StatusTeapot, "teapot")
	status, msg := MapHTTP(errSentinel400)
	if status != http.StatusTeapot {
		t.Errorf("got status %d, want %d after overwrite", status, http.StatusTeapot)
	}
	if msg != "teapot" {
		t.Errorf("got message %q, want %q after overwrite", msg, "teapot")
	}
	// Restore.
	Register(errSentinel400, http.StatusBadRequest, "test: bad request")
}
