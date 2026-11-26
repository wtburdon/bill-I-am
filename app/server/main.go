package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// The release image replaces this value with -ldflags.
var gitSHA = "dev"

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil && !errors.Is(err, flag.ErrHelp) {
		log.Print(err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	command := "serve"
	if len(args) > 0 && (args[0] == "validate" || args[0] == "serve" || args[0] == "export" || args[0] == "publish") {
		command, args = args[0], args[1:]
	}

	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(output)
	contentDir := flags.String("content-dir", "app/content", "published content directory")
	var addr, outputDir, sha *string
	if command == "serve" {
		addr = flags.String("addr", ":8080", "HTTP listen address")
	}
	if command == "export" || command == "publish" {
		outputDir = flags.String("release-dir", "", "exported release directory")
		sha = flags.String("sha", "", "full Git SHA")
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if outputDir != nil && *outputDir == "" {
		return errors.New("-release-dir is required")
	}
	if command == "publish" {
		bucket, err := bucketFromEnv()
		if err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return publishRelease(ctx, bucket, *outputDir, *sha)
	}

	var index contentIndex
	var err error
	if release := os.Getenv("R2_RELEASE_SHA"); command == "serve" && release != "" {
		if release != gitSHA {
			return fmt.Errorf("configured release %q does not match image SHA %q", release, gitSHA)
		}
		bucket, bucketErr := bucketFromEnv()
		if bucketErr != nil {
			return bucketErr
		}
		index, err = loadR2Snapshot(bucket, release)
	} else {
		index, err = loadContent(*contentDir)
	}
	if err != nil {
		return err
	}
	if command == "validate" {
		fmt.Fprintln(output, "Content validated.")
		return nil
	}

	if command == "export" {
		return exportRelease(index, *contentDir, *outputDir, *sha)
	}
	return serve(*addr, newSite(index, gitSHA), log.New(output, "", log.LstdFlags))
}

func serve(addr string, handler *site, logger *log.Logger) error {
	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          logger,
	}
	exited := make(chan error, 1)
	logger.Printf("Listening on %s (git SHA %s)", listener.Addr(), handler.gitSHA)
	go func() {
		exited <- server.Serve(listener)
	}()

	select {
	case err := <-exited:
		return err
	case <-signals.Done():
		handler.draining.Store(true)
		stop()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}
