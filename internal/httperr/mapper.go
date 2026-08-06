// Package httperr provides a centralized mapping from domain errors to HTTP
// status codes and user-facing messages. Domain packages register their
// sentinel errors via Register(). Callers use MapHTTP() to convert any error
// to an HTTP status code and message.
//
// Adding a new error type means calling Register() from the domain package
// and ensuring the httperr test covers it.
package httperr

import (
	"errors"
	"net/http"
	"sync"
)

type statusMessage struct {
	status  int
	message string
}

var (
	mu       sync.RWMutex
	errorMap = map[error]statusMessage{}
)

// Register associates a sentinel error with an HTTP status code and
// user-facing message. It should be called during package initialization
// (e.g., from an init() function or a dedicated registration file in the
// domain package). Panics if err is nil.
func Register(err error, status int, message string) {
	if err == nil {
		panic("httperr.Register: err must not be nil")
	}
	mu.Lock()
	errorMap[err] = statusMessage{status: status, message: message}
	mu.Unlock()
}

// MapHTTP converts a domain error to an HTTP status code and user-facing
// message. It walks the registered sentinel errors and checks via errors.Is.
// Unknown errors default to 500.
func MapHTTP(err error) (status int, message string) {
	mu.RLock()
	defer mu.RUnlock()
	for sentinel, sm := range errorMap {
		if errors.Is(err, sentinel) {
			return sm.status, sm.message
		}
	}
	return http.StatusInternalServerError, "internal server error"
}
