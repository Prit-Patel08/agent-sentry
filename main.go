package main

import (
	"log"

	"flowforge/cmd"
)

func main() {
	// keep main tiny; cmd.Execute implements CLI and server bootstrap
	if err := cmd.Execute(); err != nil {
		log.Fatalf("flowforge: %v", err)
	}
}
