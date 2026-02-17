package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	client := &http.Client{Timeout: 2 * time.Second}
	_, err := client.Get("http://localhost:8080/healthz")
	if err != nil {
		fmt.Printf("Healthcheck failed: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
