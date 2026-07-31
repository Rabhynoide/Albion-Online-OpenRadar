package hub

import (
	"crypto/subtle"
	"net/http"
)

// SecretHeader is the header clients must set to authenticate against the Hub.
const SecretHeader = "X-Hub-Secret"

// requireSecret wraps h, rejecting requests whose SecretHeader doesn't match secret.
// secret must be non-empty; callers should refuse to start the Hub otherwise.
func requireSecret(secret string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get(SecretHeader)
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}
