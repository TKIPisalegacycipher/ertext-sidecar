// Command ertext-sidecar is Ertext's Google Messages backend. Ertext starts it and talks to it
// over stdio; see proto/ertext/v1/ertext.proto. AGPL-3.0.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/rs/zerolog"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gm"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/server"
)

var version = "dev"

func main() {
	// Frames own stdout. Point os.Stdout at stderr so a stray print anywhere can't corrupt them.
	frames := os.Stdout
	os.Stdout = os.Stderr

	level, err := zerolog.ParseLevel(os.Getenv("ERTEXT_SIDECAR_LOG"))
	if err != nil || level == zerolog.NoLevel {
		level = zerolog.InfoLevel
	}
	log := zerolog.New(os.Stderr).Level(level).With().Timestamp().Logger()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	srv := server.New(gm.NewFactory(log), log, version, gm.LibgmVersion())
	if err := srv.Run(ctx, os.Stdin, frames); err != nil {
		log.Error().Err(err).Msg("sidecar stopped")
		os.Exit(1)
	}
}
