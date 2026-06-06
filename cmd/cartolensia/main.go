package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/app"
)

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	configPath := flag.String("config", "", "config file path")
	flag.Parse()

	if *versionFlag {
		log.Printf("cartolensia %s", app.Version)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cartolensia, err := app.New(ctx, *configPath)
	if err != nil {
		log.Fatalf("startup failed: %v", err)
	}
	defer cartolensia.Close()

	server := &http.Server{
		Addr:              cartolensia.Config.HTTP.Addr,
		Handler:           cartolensia.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("cartolensia listening on %s using %s store", cartolensia.Config.HTTP.Addr, cartolensia.StoreBackend)
		errCh <- server.ListenAndServe()
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stopCh:
		log.Printf("received %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Fatalf("shutdown failed: %v", err)
		}
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	}
}
