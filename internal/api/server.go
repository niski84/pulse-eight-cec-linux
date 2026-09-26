package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/niski84/pulse-eight-cec-linux/web"
)

// NewServer builds and returns the HTTP mux.
func NewServer() http.Handler {
	mux := http.NewServeMux()

	// Health check — reports that the service is up. The comment above each
	// route doubles as its description in scene-runner's endpoint browser, so
	// keep one short sentence here describing what every endpoint does.
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "pulse-eight-cec-linux"})
	})

	// Browser-tab icon. Served from the embedded web/ tree so the app shows a
	// branded favicon out of the box; replace web/favicon.svg to customize.
	mux.HandleFunc("GET /favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		b, err := web.FS.ReadFile("favicon.svg")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(b) //nolint:errcheck
	})

	return mux
}

func respondJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		fmt.Printf("[pulse-eight-cec-linux] encode error: %v\n", err)
	}
}
