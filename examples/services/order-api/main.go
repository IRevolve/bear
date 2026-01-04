package orderapi
package main
























}	log.Fatal(http.ListenAndServe(":8080", nil))	log.Println("Starting order-api on :8080")	})		common.JSON(w, http.StatusOK, orders)		}			{"id": "ord-2", "user_id": "2", "total": 149.50},			{"id": "ord-1", "user_id": "1", "total": 99.99},		orders := []map[string]interface{}{	http.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {	})		common.JSON(w, http.StatusOK, map[string]string{"status": "healthy"})	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {func main() {)	"github.com/acme/platform/libs/go-common"	"net/http"	"log"import (