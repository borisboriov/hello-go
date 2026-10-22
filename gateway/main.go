package main

import (
	"log"
	"net/http"
	"time"

	"github.com/borisboriov/hello-go/gateway/internal/api"
)

func main() {
	server := &http.Server{
		Addr:              ":8080",
		Handler:           api.NewHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Println("Gateway service started on :8080")
	log.Fatal(server.ListenAndServe())
}
