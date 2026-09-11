// Command calculator_server serves the calculator and Prometheus metrics.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"calculator/internal/calculator"
	"calculator/internal/config"
	"calculator/internal/httpapi"
	"calculator/internal/metrics"
	"calculator/internal/native"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	dir := filepath.Dir(executable)
	host := flag.String("host", "0.0.0.0", "listen address")
	port := flag.Int("port", 8080, "listen port")
	cPath := flag.String("c-lib", filepath.Join(dir, "libcalculator.so"), "C shared library")
	rPath := flag.String("rust-lib", filepath.Join(dir, "libcalculator_rust.so"), "Rust shared library")
	interval := flag.Float64("interval", 5, "seconds between reports")
	parallel := flag.Int("concurrency", runtime.GOMAXPROCS(0), "maximum concurrent native calculations")
	flag.Parse()
	if flag.NArg() != 0 || *port < 1 || *port > 65535 || *parallel < 1 {
		return fmt.Errorf("invalid arguments: port must be 1..65535 and concurrency positive")
	}
	period, err := config.Seconds(*interval, false)
	if err != nil {
		return fmt.Errorf("interval: %w", err)
	}
	lib, err := native.Open(*cPath, *rPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := lib.Close(); err != nil {
			log.Print(err)
		}
	}()
	stats := metrics.New()
	calc := calculator.New(lib, stats, *parallel)
	server := &http.Server{
		Addr: net.JoinHostPort(*host, strconv.Itoa(*port)), Handler: httpapi.New(calc, stats),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	log.Printf("Calculator server listening on %s; native concurrency=%d", listener.Addr(), *parallel)
	printTotals := func(label string) { sum, sub := calc.Totals(); log.Printf("[%s] sum=%d sub=%d", label, sum, sub) }
	for {
		select {
		case <-ticker.C:
			printTotals("periodic")
		case err := <-done:
			// Even a listener failure can leave active requests using the libraries.
			if shutdownErr := server.Shutdown(context.Background()); shutdownErr != nil {
				return fmt.Errorf("shutdown: %w", shutdownErr)
			}
			printTotals("final")
			if !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("serve: %w", err)
			}
			return nil
		case <-ctx.Done():
			log.Print("Stopping admission; waiting for active requests")
			// Do not dlclose while any cgo call is active. Native functions are finite;
			// HTTP read/write deadlines bound slow clients. Shutdown waits for handlers.
			if err := server.Shutdown(context.Background()); err != nil {
				return fmt.Errorf("shutdown: %w", err)
			}
			serveErr := <-done
			printTotals("final")
			if !errors.Is(serveErr, http.ErrServerClosed) {
				return fmt.Errorf("serve: %w", serveErr)
			}
			return nil
		}
	}
}
