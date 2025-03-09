package main

import (
	"log"
	"rdialer"
)

func main() {
	server := rdialer.NewServer("0.0.0.0:8443", rdialer.WithHandleFuncPattern("/connect"))
	err := server.Start()
	if err != nil {
		log.Fatal(err)
	}
}
