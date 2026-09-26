package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func serveHTTP(port string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "pulse-eight-cec"})
	})
	fmt.Printf("[pulse-eight-cec] ready at http://localhost:%s\n", port)
	return http.ListenAndServe(":"+port, mux)
}
