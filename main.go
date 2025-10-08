package main

import (
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"rueDeLegliseBooker/internal/server"
	"rueDeLegliseBooker/internal/storage"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

func main() {
	cfg := loadConfig("config.json")
	people := assignColours(cfg.People)

	if err := os.MkdirAll("data", 0o755); err != nil {
		log.Fatalf("unable to ensure data directory: %v", err)
	}

	store, err := storage.New(filepath.Join("data", "reservations.db"))
	if err != nil {
		log.Fatalf("failed to initialise storage: %v", err)
	}
	defer store.Close()

	tpl, err := template.ParseFS(templateFS, "templates/index.html")
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}

	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("failed to load static assets: %v", err)
	}
	staticHandler := http.StripPrefix("/static/", http.FileServer(http.FS(staticContent)))

	srv := server.New(store, tpl, staticHandler, people, cfg.PageTitle, cfg.BannerTitle, cfg.BasePath)

	addr := ":64512"
	log.Printf("Service lance sur http://localhost%s", addr)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func assignColours(names []string) []server.Person {
	palette := []string{
		"#800000",
		"#3cb44b",
		"#ffe119",
		"#0082c8",
		"#f58231",
		"#911eb4",
		"#46f0f0",
		"#f032e6",
		"#d2f53c",
		"#fabebe",
		"#008080",
		"#e6beff",
		"#aa6e28",
		"#fffac8",
		"#aaffc3",
		"#808000",
		"#ffd8b1",
		"#000080",
		"#808080",
	}

	people := make([]server.Person, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		color := palette[len(people)%len(palette)]
		people = append(people, server.Person{
			Name:  name,
			Color: color,
		})
	}
	return people
}

type appConfig struct {
	People      []string `json:"people"`
	PageTitle   string   `json:"page_title"`
	BannerTitle string   `json:"banner_title"`
	BasePath    string   `json:"base_path"`
}

func loadConfig(path string) appConfig {
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("warning: unable to read config %q: %v (using defaults)", path, err)
		}
		return cfg
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("warning: unable to parse config %q: %v (using defaults)", path, err)
		return defaultConfig()
	}

	applyConfigDefaults(&cfg)
	cfg.BasePath = sanitiseBasePath(cfg.BasePath)
	return cfg
}

func defaultConfig() appConfig {
	return appConfig{
		People: []string{
			"Annabelle",
			"Florence",
			"Gregoire",
			"Manon",
			"Valentin",
			"Yves",
		},
		PageTitle:   "Reservations appartement",
		BannerTitle: "Planning des 18 prochains mois",
		BasePath:    "",
	}
}

func applyConfigDefaults(cfg *appConfig) {
	def := defaultConfig()
	if len(cfg.People) == 0 {
		cfg.People = def.People
	}
	if cfg.PageTitle == "" {
		cfg.PageTitle = def.PageTitle
	}
	if cfg.BannerTitle == "" {
		cfg.BannerTitle = def.BannerTitle
	}
}

func sanitiseBasePath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "/" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	trimmed = strings.TrimRight(trimmed, "/")
	return trimmed
}
