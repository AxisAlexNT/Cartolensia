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
	"strings"
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

	errCh := make(chan error, 1)
	handler := cartolensia.Handler()
	var servers []*http.Server
	startServer := func(server *http.Server, serve func() error) {
		servers = append(servers, server)
		go func() {
			if err := serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()
	}

	tlsEnabled := (cartolensia.Config.HTTP.TLSCertFile != "" && cartolensia.Config.HTTP.TLSKeyFile != "") || cartolensia.Config.HTTP.TLSAutoSelfSigned
	if tlsEnabled {
		tlsAddr := cartolensia.Config.HTTP.TLSAddr
		if tlsAddr == "" {
			tlsAddr = cartolensia.Config.HTTP.Addr
		}
		tlsServer := &http.Server{
			Addr:              tlsAddr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		}
		if cartolensia.Config.HTTP.TLSCertFile != "" && cartolensia.Config.HTTP.TLSKeyFile != "" {
			log.Printf("cartolensia listening with configured TLS on %s using %s store", tlsAddr, cartolensia.StoreBackend)
			startServer(tlsServer, func() error {
				return tlsServer.ListenAndServeTLS(cartolensia.Config.HTTP.TLSCertFile, cartolensia.Config.HTTP.TLSKeyFile)
			})
		} else {
			cert, certPath, err := tlsutil.LoadOrCreateSelfSignedCertificate(cartolensia.Config.Cache.Dir, tlsHosts([]string{cartolensia.Config.HTTP.Addr, tlsAddr}, cartolensia.Config.HTTP.TLSHosts))
			if err != nil {
				log.Fatalf("load or generate self-signed certificate: %v", err)
			}
			tlsServer.TLSConfig = &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: []tls.Certificate{cert},
			}
			if certPath != "" {
				log.Printf("cartolensia listening with cached self-signed TLS on %s using %s store (cert %s)", tlsAddr, cartolensia.StoreBackend, certPath)
			} else {
				log.Printf("cartolensia listening with in-memory self-signed TLS on %s using %s store", tlsAddr, cartolensia.StoreBackend)
			}
			startServer(tlsServer, func() error {
				return tlsServer.ListenAndServeTLS("", "")
			})
		}
		if cartolensia.Config.HTTP.RedirectHTTPToHTTPS && cartolensia.Config.HTTP.Addr != "" && cartolensia.Config.HTTP.Addr != tlsAddr {
			redirectServer := &http.Server{
				Addr:              cartolensia.Config.HTTP.Addr,
				Handler:           httpsRedirectHandler(tlsAddr, handler),
				ReadHeaderTimeout: 5 * time.Second,
			}
			log.Printf("cartolensia redirecting HTTP on %s to HTTPS on %s", cartolensia.Config.HTTP.Addr, tlsAddr)
			startServer(redirectServer, redirectServer.ListenAndServe)
		}
	} else {
		server := &http.Server{
			Addr:              cartolensia.Config.HTTP.Addr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		}
		log.Printf("cartolensia listening on %s using %s store", cartolensia.Config.HTTP.Addr, cartolensia.StoreBackend)
		startServer(server, server.ListenAndServe)
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stopCh:
		log.Printf("received %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, server := range servers {
			if err := server.Shutdown(ctx); err != nil {
				log.Fatalf("shutdown failed: %v", err)
			}
		}
	case err := <-errCh:
		log.Fatalf("server failed: %v", err)
	}
}

func tlsHosts(addrs []string, configured []string) []string {
	hosts := append([]string(nil), configured...)
	for _, candidate := range addrs {
		host, _, err := net.SplitHostPort(candidate)
		if err == nil && host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

func httpsRedirectHandler(tlsAddr string, bypass http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/ai-media/") && bypass != nil {
			bypass.ServeHTTP(w, r)
			return
		}
		host := r.Host
		if requestHost, _, err := net.SplitHostPort(r.Host); err == nil {
			host = requestHost
		}
		if tlsHost, tlsPort, err := net.SplitHostPort(tlsAddr); err == nil {
			if tlsHost != "" && tlsHost != "0.0.0.0" && tlsHost != "::" {
				host = tlsHost
			}
			if tlsPort != "" && tlsPort != "443" {
				host = net.JoinHostPort(host, tlsPort)
			}
		}
		target := "https://" + host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
	})
}
