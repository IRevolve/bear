package main

import (
"encoding/json"
"fmt"
"log"
"net/http"
)

type GitHubEvent struct {
	Action     string `json:"action"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	var event GitHubEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	log.Printf("Received %s event for %s", event.Action, event.Repository.FullName)

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status": "ok"}`)
}

func main() {
	http.HandleFunc("/webhook", handler)
	log.Println("Starting github-webhook on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
