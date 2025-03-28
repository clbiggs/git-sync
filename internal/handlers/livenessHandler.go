package handlers

import (
	"net/http"
)

//nolint:errcheck // no need to check write error
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}
