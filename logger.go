package rdialer

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
	"path/filepath"
	"strconv"
)

var InitDefaultLogger bool = true

func init() {
	if InitDefaultLogger {
		// Customize CallerMarshalFunc to display only the file name
		zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
			return filepath.Base(file) + ":" + strconv.Itoa(line)
		}
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			NoColor:    false,
			TimeFormat: "2006-01-02 15:04:05.000-0700",
		}).With().Caller().Timestamp().Logger()
	}
}
