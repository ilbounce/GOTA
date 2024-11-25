package main

import (
	"log"

	"tarbitrage/internal/app/apiserver"
)

func main() {
	if err := apiserver.Start(); err != nil {
		log.Fatal(err)
	}
}
