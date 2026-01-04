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

	http.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
orders := []map[string]interface{}{
{"id": "ord-1", "user_id": "1", "total": 99.99},
{"id": "ord-2", "user_id": "2", "total": 149.50},
}
w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(orders)
	})

	log.Println("Starting order-api on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
