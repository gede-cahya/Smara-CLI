package web

import (
	"net/http"
	"strings"
)

func (s *Server) withAuth(next http.Handler) http.Handler {
	if s.AuthToken == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || r.URL.Path == "/api/status" {
			next.ServeHTTP(w, r)
			return
		}
		token := r.URL.Query().Get("token")
		if token == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				token = strings.TrimSpace(auth[7:])
			}
		}
		if token != s.AuthToken {
			errorResponse(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}
