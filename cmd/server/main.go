// Command server is the entry point for the URL shortener API.
//
// In Go, an executable lives in `package main` and starts at func main().
// There is no framework bootstrapping like Laravel's public/index.php — this
// file IS the application's starting point.
package main

import (
	"log"
	"net/http"
)

func main() {
	// http.ServeMux is Go's built-in HTTP router. Since Go 1.22 it understands
	// method + path patterns like "GET /health", so we need no framework.
	mux := http.NewServeMux()

	// Our first route. A handler is just a function with this exact signature:
	//   func(w http.ResponseWriter, r *http.Request)
	// w is where you write the response; r holds the incoming request.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Write sends the body. We didn't call WriteHeader, so status is 200.
		w.Write([]byte(`{"status":"ok"}`))
	})

	const addr = ":8080"
	log.Printf("listening on http://localhost%s", addr)

	// ListenAndServe starts the server and blocks forever. It only returns if
	// something goes wrong (e.g. port already in use), and then we crash loudly.
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
