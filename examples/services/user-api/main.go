package main

import (
	"log"
	"net/http"

	"github.com/acme/platform/libs/go-common"
)

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		common.JSON(w, http.StatusOK, map[string]string{"status": "healthy"})
	})

	http.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		users := []map[string]string{
			{"id": "1", "name": "Alice"},
			{"id": "2", "name": "Bob"},
		}
		common.JSON(w, http.StatusOK, users)
	})

	log.Println("Starting user-api on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
