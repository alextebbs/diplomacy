package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/sammy/diplomacy/internal/bot"
	"github.com/sammy/diplomacy/internal/db"
	"github.com/sammy/diplomacy/internal/game"
)

func main() {
	bi := computeBuildInfo()
	log.Printf("Starting diplomacy bot binary=%s go=%s", bi.BinaryHash, bi.GoVersion)
	bot.SetBuildInfo(bi)

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("DISCORD_TOKEN environment variable is required")
	}

	appID := os.Getenv("DISCORD_APP_ID")
	if appID == "" {
		log.Fatal("DISCORD_APP_ID environment variable is required")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	http.HandleFunc("/checksum", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(bi.BinaryHash))
	})
	go func() {
		log.Println("Health check listening on :8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Printf("Health server error: %v", err)
		}
	}()

	store, err := db.Open(dbURL)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	mgr := game.NewManager(store)

	scheduler := game.NewScheduler(mgr, store)

	b, err := bot.New(token, appID, mgr, store, scheduler)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	if err := b.Start(); err != nil {
		log.Fatalf("Failed to start bot: %v", err)
	}
	defer b.Stop()

	log.Println("Diplomacy bot is running. Press Ctrl+C to exit.")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("Shutting down...")
}

func computeBuildInfo() bot.BuildInfo {
	bi := bot.BuildInfo{
		GoVersion:  runtime.Version(),
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		BinaryHash: "unavailable",
	}

	exePath, err := os.Executable()
	if err == nil {
		if h, err := hashFile(exePath); err == nil {
			bi.BinaryHash = h
		}
	}

	return bi
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
