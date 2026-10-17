package httpx

import (
	"log"
	"net/http"
)

// wildcardOrigin means "allow every origin". It is honoured only when an
// operator puts it in the allowlist explicitly; it is never a default.
const wildcardOrigin = "*"

// CORS returns middleware that reflects the request Origin back only when that
// origin is in allowedOrigins, which must contain at least one entry.
//
// Requests without an Origin header (curl, server-to-server calls, same-origin
// navigation) pass through untouched.
//
// A single "*" entry restores a permissive policy. Note the tradeoff before
// using it: a wildcard is incompatible with credentialed requests, but it still
// lets any website a user visits read this API's responses, and this API has no
// authentication. It must be a deliberate choice, not a default.
//
// This function is a pure configuration consumer: it reads no environment and
// makes no policy decision. platform/config owns where the list comes from.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = true
	}

	if allowed[wildcardOrigin] {
		log.Printf("CORS: wildcard origin configured; any website can read this API's responses")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			switch {
			case origin == "":
				// Not a cross-origin request; no CORS headers are needed.
			case allowed[wildcardOrigin]:
				w.Header().Set("Access-Control-Allow-Origin", wildcardOrigin)
				w.Header().Set("Vary", "Origin")
			case allowed[origin]:
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			default:
				// Unknown origin: send no CORS headers, so the browser blocks
				// the response. The request still reaches the handler, because
				// CORS is not an access control mechanism for non-browser
				// clients.
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusForbidden)
					return
				}
			}

			if r.Method == http.MethodOptions {
				if origin != "" && (allowed[origin] || allowed[wildcardOrigin]) {
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
