package middleware

import "net/http"

// JSON is a basic middleware that always sets the response content type to a valid JSON type.
func JSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	}
}
