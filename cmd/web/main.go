// Package main runs the web-based control center.
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/verssache/chatgpt-creator/internal/server"
	"github.com/verssache/chatgpt-creator/internal/store"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "listen address")
	dbPath := flag.String("db", "botdata.db", "sqlite database path")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	srv := server.New(*addr, st)

	// Graceful shutdown on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Printf("shutting down")
		// best-effort: manager stops current job via cancel.
		os.Exit(0)
	}()

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
