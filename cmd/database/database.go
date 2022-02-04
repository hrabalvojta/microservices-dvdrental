package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/hrabalvojtech/watermark-service/internal/database"
	dbsvc "github.com/hrabalvojtech/watermark-service/pkg/database"
	"github.com/hrabalvojtech/watermark-service/pkg/database/endpoints"
	"github.com/hrabalvojtech/watermark-service/pkg/database/transport"
)

const (
	defaultHTTPPort = "8081"
)

var (
	logger   log.Logger
	httpAddr = net.JoinHostPort("localhost", envString("HTTP_PORT", defaultHTTPPort))
)

func envString(env, fallback string) string {
	e := os.Getenv(env)
	if e == "" {
		return fallback
	}
	return e
}

func init() {
	logger = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)

	db, err := database.Init(
		database.DefaultHost,
		database.DefaultPort,
		database.DefaultDatabase,
		database.DefaultDBUser,
		database.DefaultPassword,
		database.DefaultSSLMode,
		database.DefaultTimeZone,
	)

	defer func() {
		err := db.Close()
		if err != nil {
			logger.Log("ERROR::Failed to close the database connection ", err.Error())
		}
	}()
	if err != nil {
		logger.Log(fmt.Sprintf("FATAL: failed to load db with error: %s", err.Error()))
	}
}

func main() {
	var (
		service     = dbsvc.NewService()
		eps         = endpoints.NewEndpointSet(service)
		httpHandler = transport.NewHTTPHandler(eps)
	)

	var g group.Group
	{
		// The HTTP listener mounts the Go kit HTTP handler we created.
		httpListener, err := net.Listen("tcp", httpAddr)
		if err != nil {
			logger.Log("transport", "HTTP", "during", "Listen", "err", err)
			os.Exit(1)
		}
		g.Add(func() error {
			logger.Log("transport", "HTTP", "addr", httpAddr)
			return http.Serve(httpListener, httpHandler)
		}, func(error) {
			httpListener.Close()
		})
	}

	{
		// This function just sits and waits for ctrl-C.
		cancelInterrupt := make(chan struct{})
		g.Add(func() error {
			c := make(chan os.Signal, 1)
			signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
			select {
			case sig := <-c:
				return fmt.Errorf("received signal %s", sig)
			case <-cancelInterrupt:
				return nil
			}
		}, func(error) {
			close(cancelInterrupt)
		})
	}
	logger.Log("exit", g.Run())
}
