package main

import (
"fmt"
"net/http"
"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
fmt.Fprintf(w, "Hello from Bear! 🐻\n")
})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusOK)
fmt.Fprintf(w, "OK\n")
})

	fmt.Printf("🐻 Hello API listening on port %s\n", port)
	http.ListenAndServe(":"+port, nil)
}
