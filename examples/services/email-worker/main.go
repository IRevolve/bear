package main

import (
	"log"
	"time"
)

func main() {
	log.Println("Email worker starting...")

	// Simulate processing email queue
	for i := 0; i < 10; i++ {
		log.Printf("Processing email batch %d/10", i+1)
		time.Sleep(100 * time.Millisecond)
	}

	log.Println("Email worker completed")
}
