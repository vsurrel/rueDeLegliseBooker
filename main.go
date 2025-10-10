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

	"AppartmentBooker/internal/server"
	"AppartmentBooker/internal/storage"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

func main() {
	cfg := loadConfig("config.json")
	people := assignColours(cfg.People)
	authCfg := loadAuthConfig("auth.json")

	if err := os.MkdirAll("data", 0o755); err != nil {
		log.Fatalf("unable to ensure data directory: %v", err)
	}

	store, err := storage.New(filepath.Join("data", "reservations.db"))
	if err != nil {
		log.Fatalf("failed to initialise storage: %v", err)
	}
	defer store.Close()

	tpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}

	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("failed to load static assets: %v", err)
	}
	staticHandler := http.StripPrefix("/static/", http.FileServer(http.FS(staticContent)))

	srv := server.New(store, tpl, staticHandler, people, cfg.PageTitle, cfg.BannerTitle, cfg.BasePath, authCfg.Password, authCfg.Hint)

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

type authConfig struct {
	Password string `json:"password"`
	Hint     string `json:"hint"`
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

func loadAuthConfig(path string) authConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Fatalf("auth configuration %q introuvable", path)
		}
		log.Fatalf("lecture de la configuration auth %q impossible: %v", path, err)
	}

	var cfg authConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("analyse de la configuration auth %q impossible: %v", path, err)
	}

	cfg.Password = strings.TrimSpace(cfg.Password)
	cfg.Hint = strings.TrimSpace(cfg.Hint)
	if cfg.Password == "" {
		log.Fatalf("la configuration auth %q doit contenir un mot de passe non vide", path)
	}

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
