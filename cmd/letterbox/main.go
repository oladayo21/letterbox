package main

import (
	"log"
	"net/http"

	"github.com/oladayo21/letterbox/internal/config"
)

func main() {
	cfg, err := config.Load()

	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	log.Printf("letterbox starting on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}
