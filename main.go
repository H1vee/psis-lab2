package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var version = "dev"

type Brewing struct {
	Tea        string `json:"tea"`
	WaterMinC  int    `json:"water_min_c"`
	WaterMaxC  int    `json:"water_max_c"`
	SecondsMin int    `json:"seconds_min"`
	SecondsMax int    `json:"seconds_max"`
	NeedsRinse bool   `json:"needs_rinse"`
}

var catalog = []Brewing{
	{Tea: "green", WaterMinC: 70, WaterMaxC: 80, SecondsMin: 60, SecondsMax: 120, NeedsRinse: false},
	{Tea: "white", WaterMinC: 80, WaterMaxC: 85, SecondsMin: 120, SecondsMax: 180, NeedsRinse: false},
	{Tea: "oolong", WaterMinC: 90, WaterMaxC: 95, SecondsMin: 60, SecondsMax: 120, NeedsRinse: true},
	{Tea: "black", WaterMinC: 90, WaterMaxC: 95, SecondsMin: 180, SecondsMax: 240, NeedsRinse: false},
	{Tea: "puer", WaterMinC: 95, WaterMaxC: 100, SecondsMin: 20, SecondsMax: 40, NeedsRinse: true},
}

func resolveAddr() string {
	addr := os.Getenv("TEAPOT_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	return addr
}

func healthURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr + "/healthz"
	}
	return "http://" + addr + "/healthz"
}

func runHealthCheck(addr string) int {
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(healthURL(addr))
	if err != nil {
		log.Printf("health check failed: %v", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("health check returned status %d", resp.StatusCode)
		return 1
	}
	return 0
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": version})
}

func handleBrewing(w http.ResponseWriter, r *http.Request) {
	requested := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tea")))

	result := catalog
	if requested != "" {
		result = nil
		for _, item := range catalog {
			if item.Tea == requested {
				result = append(result, item)
			}
		}
		if len(result) == 0 {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "unknown tea type"})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-App-Version", version)
	json.NewEncoder(w).Encode(result)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="uk">
<head><meta charset="utf-8"><title>Teapot API</title></head>
<body style="font-family: system-ui, sans-serif; max-width: 40rem; margin: 3rem auto; line-height: 1.6">
<h1>Teapot API</h1>
<p>Версія сервісу: <strong>%s</strong>. Хост контейнера: <strong>%s</strong>.</p>
<ul>
<li><a href="/api/brewing">/api/brewing</a> — усі параметри заварювання</li>
<li><a href="/api/brewing?tea=oolong">/api/brewing?tea=oolong</a> — фільтр за типом чаю</li>
<li><a href="/healthz">/healthz</a> — стан сервісу</li>
</ul>
</body>
</html>`, version, hostname())
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return name
}

func newServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/brewing", handleBrewing)
	mux.HandleFunc("/healthz", handleHealth)

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func main() {
	addr := flag.String("addr", resolveAddr(), "address the HTTP server listens on")
	healthcheck := flag.Bool("healthcheck", false, "probe the running server and exit")
	flag.Parse()

	if *healthcheck {
		os.Exit(runHealthCheck(*addr))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server := newServer(*addr)

	go func() {
		log.Printf("teapot %s listening on %s", version, *addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server stopped with error: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Print("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
		os.Exit(1)
	}
	log.Print("server stopped cleanly")
}
