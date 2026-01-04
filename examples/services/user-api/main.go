package main

import (
"encoding/json"
"log"
"net/http"
)

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	http.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
users := []map[string]string{
{"id": "1", "name": "Alice"},
{"id": "2", "name": "Bob"},
}
w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	})

	log.Println("Starting user-api on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
