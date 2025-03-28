package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/clbiggs/git-sync/pkg/git/syncer"
)

func StatusHandler(sync *syncer.Syncer) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(sync.Status())
	}
}
