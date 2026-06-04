package main

import (
	"flag"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/pipe01/flydigictl/pkg/dbus/server"
	"github.com/pipe01/flydigictl/pkg/flydigi"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	golog "log"
)

func logError(msg string, closer func() error) {
	if err := closer(); err != nil {
		log.Err(err).Msg(msg)
	}
}

func main() {
	prettyLogging := flag.Bool("pretty-logs", false, "Enable human-readable colored logs")
	useSessionBus := flag.Bool("session-bus", false, "Use DBus session bus instead of system (not recommended)")
	flag.Parse()

	if *prettyLogging {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	gp, err := flydigi.OpenGamepad()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open gamepad")
	}
	defer logError("failed to close gamepad", gp.Close)

	// p := true
	// for range time.Tick(1 * time.Second) {
	// 	err := ug.Key(evdev.BTN_A, p)
	// 	if err != nil {
	// 		log.Err(err).Msg("failed to write event")
	// 	}
	// 	p = !p
	// }

	golog.SetOutput(io.Discard) // Supress github.com/google/gousb logging

	srv := server.New()

	if err := srv.Listen(*useSessionBus); err != nil {
		log.Fatal().Err(err).Msg("failed to start dbus server")
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	sig := <-ch

	log.Info().Stringer("signal", sig).Msg("exiting")
}
