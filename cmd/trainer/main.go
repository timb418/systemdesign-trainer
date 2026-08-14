package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	traineragent "github.com/timb418/systemdesign-trainer/internal/agent"
	"github.com/timb418/systemdesign-trainer/internal/settings"
	"github.com/timb418/systemdesign-trainer/internal/store"
	"github.com/timb418/systemdesign-trainer/internal/tasks"
	"github.com/timb418/systemdesign-trainer/internal/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	fsys, err := tasks.Embedded()
	if err != nil {
		return err
	}
	bank, err := tasks.Load(fsys)
	if err != nil {
		return fmt.Errorf("банк задач: %w", err)
	}
	set, err := settings.Open()
	if err != nil {
		return err
	}
	dataDir, err := settings.DataDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	st, err := store.Open(filepath.Join(dataDir, "sessions.db"))
	if err != nil {
		return err
	}
	defer st.Close()
	agents := traineragent.New(bank, set)
	srv, err := web.New(bank, st, set, agents)
	if err != nil {
		return err
	}
	addr := os.Getenv("SDT_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	return web.ListenAndServe(addr, srv.Handler())
}
