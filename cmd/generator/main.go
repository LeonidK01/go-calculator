// Command generator sends concurrent POST requests to the calculator.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"calculator/internal/config"
	"calculator/internal/generator"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	endpoint := flag.String("url", "http://localhost:8080/calc", "calculator endpoint")
	var workers int
	flag.IntVar(&workers, "threads", 10, "number of concurrent workers")
	flag.IntVar(&workers, "n", 10, "alias for --threads")
	interval := flag.Float64("interval", 0.1, "pause per worker in seconds (0 = maximum throughput)")
	timeout := flag.Float64("timeout", 5, "HTTP timeout in seconds")
	duration := flag.Float64("duration", 0, "run duration in seconds (0 = until signal)")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	pause, err := config.Seconds(*interval, true)
	if err != nil {
		return fmt.Errorf("interval: %w", err)
	}
	requestTimeout, err := config.Seconds(*timeout, false)
	if err != nil {
		return fmt.Errorf("timeout: %w", err)
	}
	limit, err := config.Seconds(*duration, true)
	if err != nil {
		return fmt.Errorf("duration: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if limit > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, limit)
		defer cancel()
	}
	start := time.Now()
	log.Printf("Generator started: %d workers -> %s", workers, *endpoint)
	stats, err := generator.Run(ctx, generator.Config{URL: *endpoint, Workers: workers, Interval: pause, Timeout: requestTimeout})
	if err != nil {
		return err
	}
	elapsed := time.Since(start).Seconds()
	log.Printf("Total requests: ok=%d errors=%d elapsed=%.3fs rps=%.2f", stats.OK, stats.Errors, elapsed, float64(stats.OK+stats.Errors)/elapsed)
	return nil
}
