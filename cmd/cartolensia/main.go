package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/app"
	"github.com/AxisAlexNT/Cartolensia/internal/tlsutil"
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
		if cartolensia.Config.HTTP.TLSCertFile != "" && cartolensia.Config.HTTP.TLSKeyFile != "" {
			log.Printf("cartolensia listening with configured TLS on %s using %s store", cartolensia.Config.HTTP.Addr, cartolensia.StoreBackend)
			errCh <- server.ListenAndServeTLS(cartolensia.Config.HTTP.TLSCertFile, cartolensia.Config.HTTP.TLSKeyFile)
			return
		}
		if cartolensia.Config.HTTP.TLSAutoSelfSigned {
			cert, err := tlsutil.SelfSignedCertificate(tlsHosts(cartolensia.Config.HTTP.Addr, cartolensia.Config.HTTP.TLSHosts))
			if err != nil {
				errCh <- err
				return
			}
			server.TLSConfig = &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: []tls.Certificate{cert},
			}
			log.Printf("cartolensia listening with generated self-signed TLS on %s using %s store", cartolensia.Config.HTTP.Addr, cartolensia.StoreBackend)
			errCh <- server.ListenAndServeTLS("", "")
			return
		}
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

func tlsHosts(addr string, configured []string) []string {
	hosts := append([]string(nil), configured...)
	host, _, err := net.SplitHostPort(addr)
	if err == nil && host != "" {
		hosts = append(hosts, host)
	}
	return hosts
}
