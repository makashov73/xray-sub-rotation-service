package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// RequestID generates a unique ID for each request.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := make([]byte, 16)
		rand.Read(id)
		requestID := hex.EncodeToString(id)
		w.Header().Set("X-Request-Id", requestID)
		next.ServeHTTP(w, r)
	})
}
