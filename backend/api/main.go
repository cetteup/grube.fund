package main

import (
	"fmt"
	"os"
	"time"

	"github.com/cetteup/grube.fund/backend/api/internal/config"
	"github.com/cetteup/grube.fund/backend/api/internal/feeds"
	"github.com/cetteup/grube.fund/internal/feed"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	buildVersion = "development"
	buildCommit  = "uncommitted"
	buildTime    = "unknown"
)

func main() {
	version := fmt.Sprintf("grube.fund api %s (%s) built at %s", buildVersion, buildCommit, buildTime)
	cfg := config.Init()

	// Print version and exit
	if cfg.Version {
		fmt.Println(version)
		os.Exit(0)
	}

	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		NoColor:    !cfg.ColorizeLogs,
		TimeFormat: time.RFC3339,
	})
	if cfg.Debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	saturnGenerator := feed.NewGenerator("Saturn", "https://www.saturn.de/de/data/fundgrube/api/postings", "https://www.saturn.de/de/data/fundgrube")
	mediamarktGenerator := feed.NewGenerator("MediaMarkt", "https://www.mediamarkt.de/de/data/fundgrube/api/postings", "https://www.mediamarkt.de/de/data/fundgrube")
	saturnHandler := feeds.NewHandler(saturnGenerator)
	mediamarktHandler := feeds.NewHandler(mediamarktGenerator)

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Recover())
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: time.Second * 10,
	}))
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogError:     true,
		LogRemoteIP:  true,
		LogURI:       true,
		LogStatus:    true,
		LogLatency:   true,
		LogUserAgent: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			log.Info().
				Err(v.Error).
				Str("remote", v.RemoteIP).
				Str("URI", v.URI).
				Int("status", v.Status).
				Str("latency", v.Latency.Truncate(time.Millisecond).String()).
				Str("agent", v.UserAgent).
				Msg("request")

			return nil
		},
	}))

	e.GET("/feed/v1/saturn/:format", saturnHandler.HandleGet)
	e.GET("/feed/v1/mediamarkt/:format", mediamarktHandler.HandleGet)

	log.Info().Str("listenAddr", cfg.ListenAddr).Msg("Starting HTTP server")
	log.Fatal().Err(e.Start(cfg.ListenAddr))
}
