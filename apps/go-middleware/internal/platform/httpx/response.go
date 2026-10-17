// Package httpx holds transport-level helpers shared by every bounded context's
// HTTP layer: JSON encoding, the error envelope, and middleware.
//
// It knows about net/http and about platform/apperror. It knows nothing about
// any individual context's domain.
package httpx

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
)

// ErrorResponse is the standard JSON error envelope for every endpoint.
type ErrorResponse struct {
	Error string `json:"error"`
}

// WriteJSON encodes payload as JSON with the given status code.
// An encoding failure is logged after the header is already sent, so it cannot
// be reported to the client.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("error encoding response: %v", err)
	}
}

// WriteError writes an ErrorResponse with the given status code and message.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorResponse{Error: message})
}

// WriteErrorFrom maps a sentinel error to an HTTP status and writes the
// matching error envelope. Unrecognized errors are logged and reported as 500
// so internal details never leak to the client.
func WriteErrorFrom(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperror.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, apperror.ErrConflict):
		WriteError(w, http.StatusConflict, "conflict")
	case errors.Is(err, apperror.ErrInvalidInput):
		WriteError(w, http.StatusBadRequest, "invalid input")
	case errors.Is(err, apperror.ErrTooLarge):
		WriteError(w, http.StatusRequestEntityTooLarge, "payload too large")
	default:
		log.Printf("internal error: %v", err)
		WriteError(w, http.StatusInternalServerError, "internal server error")
	}
}
