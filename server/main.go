package main

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"rdialer"
)

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	server := rdialer.NewServer("0.0.0.0:8443", rdialer.WithHandleFuncPattern("/connect"))
	err := server.Start()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start server")
	}
}
