package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	KnowledgeRepoURL     string
	KnowledgeRepoPath    string
	KnowledgeRepoRef     string
	Port                 string
	InternalServiceToken string
	// DatabaseURL is optional because Content currently serves Git-backed
	// knowledge and does not require a database for normal requests. When it is
	// configured, /readyz performs a one-shot probe against this single active
	// primary; it is not a second runtime database or an automatic fallback.
	DatabaseURL        string
	SupabaseConnectURI string
	ReloadInterval     time.Duration
}

func Load() (Config, error) {
	appEnv := strings.TrimSpace(os.Getenv("APP_ENV"))
	if appEnv == "" {
		appEnv = "dev"
	}

	// Progressively load env files, allowing overrides from more specific ones.
	_ = godotenv.Load(".env." + appEnv + ".local")
	_ = godotenv.Load(".env." + appEnv)
	_ = godotenv.Load() // fallback to .env

	cfg := Config{
		KnowledgeRepoURL:     strings.TrimSpace(os.Getenv("KNOWLEDGE_REPO_URL")),
		KnowledgeRepoPath:    strings.TrimSpace(os.Getenv("KNOWLEDGE_REPO_PATH")),
		KnowledgeRepoRef:     strings.TrimSpace(os.Getenv("KNOWLEDGE_REPO_REF")),
		Port:                 strings.TrimSpace(os.Getenv("DOCS_SERVICE_PORT")),
		InternalServiceToken: strings.TrimSpace(os.Getenv("INTERNAL_SERVICE_TOKEN")),
		DatabaseURL:          databaseURLFromEnv(),
		ReloadInterval:       5 * time.Minute,
	}
	cfg.SupabaseConnectURI = cfg.DatabaseURL

	if cfg.KnowledgeRepoPath == "" {
		return Config{}, fmt.Errorf("KNOWLEDGE_REPO_PATH is required")
	}
	if cfg.KnowledgeRepoRef == "" {
		cfg.KnowledgeRepoRef = "main"
	}
	if cfg.Port == "" {
		cfg.Port = "8084"
	}

	if raw := strings.TrimSpace(os.Getenv("DOCS_RELOAD_INTERVAL")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			cfg.ReloadInterval = parsed
		} else if seconds, convErr := strconv.Atoi(raw); convErr == nil {
			cfg.ReloadInterval = time.Duration(seconds) * time.Second
		} else {
			return Config{}, fmt.Errorf("invalid DOCS_RELOAD_INTERVAL: %w", err)
		}
	}

	return cfg, nil
}

func supabaseConnectURIFromEnv() string {
	if value := strings.TrimSpace(os.Getenv("SUPABASE_CONNECT_URI")); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("SUPABASE_CONNECT_URL"))
}

func databaseURLFromEnv() string {
	if value := supabaseConnectURIFromEnv(); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("DATABASE_URL"))
}
