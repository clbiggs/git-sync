package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/clbiggs/git-sync/pkg/git/syncer"
)

func WebhookHandler(sync *syncer.Syncer) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		log.Println("Webhook triggered: forcing pull")
		err := sync.ForceSync()
		if err != nil {
			details := map[string]any{
				"error":  err.Error(),
				"status": sync.Status(),
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(details)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(sync.Status())
	}
}
