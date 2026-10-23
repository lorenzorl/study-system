package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
)

// readJSONBody decodes the request's JSON body under a byte ceiling, answering
// the request itself when it cannot, and reports whether a payload was decoded.
//
// Both document-creation routes share it: the ceiling they enforce is the same
// STUDY_MAX_UPLOAD_BYTES the artifact store applies, and the two refusals it
// produces — 413 naming the bound, 400 carrying the decoder's reason — must not
// differ between two endpoints that accept the same kind of client.
func readJSONBody(w http.ResponseWriter, r *http.Request, limit int64, payload any) bool {
	// An announced length over the ceiling is refused before a byte is read: a
	// client sending a book has nothing to gain from watching it upload first.
	if r.ContentLength > limit {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, bodyCeilingMessage(limit))
		return false
	}

	// MaxBytesReader is the streaming half of the same ceiling: a body that lies
	// about its length, or sends none, cannot make the decoder buffer past the
	// limit. The body is therefore never held in memory beyond it.
	body := http.MaxBytesReader(w, r.Body, limit)

	if err := json.NewDecoder(body).Decode(payload); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, bodyCeilingMessage(limit))
			return false
		}
		// The decoder's own reason, not a generic message: a client that sent a
		// trailing comma should be able to see where.
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}

	return true
}

// bodyCeilingMessage names the bound, so a caller knows what to trim rather than
// only that it was refused.
func bodyCeilingMessage(limit int64) string {
	return fmt.Sprintf("the request body must be at most %d bytes", limit)
}
