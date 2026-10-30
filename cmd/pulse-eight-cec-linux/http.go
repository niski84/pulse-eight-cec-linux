package main

import (
	"fmt"
	"net"
	"net/http"

	"github.com/niski84/pulse-eight-cec-linux/internal/api"
)

func serveHTTP(port string) error {
	fmt.Printf("[pulse-eight-cec] ready at http://localhost:%s\n", port)
	return http.ListenAndServe(net.JoinHostPort("127.0.0.1", port), api.NewServer())
}
