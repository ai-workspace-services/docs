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
	KnowledgeRepoPath    string
	Port                 string
	InternalServiceToken string
	ReloadInterval       time.Duration
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
		KnowledgeRepoPath:    strings.TrimSpace(os.Getenv("KNOWLEDGE_REPO_PATH")),
		Port:                 strings.TrimSpace(os.Getenv("DOCS_SERVICE_PORT")),
		InternalServiceToken: strings.TrimSpace(os.Getenv("INTERNAL_SERVICE_TOKEN")),
		ReloadInterval:       5 * time.Minute,
	}

	if cfg.KnowledgeRepoPath == "" {
		cfg.KnowledgeRepoPath = "/Users/shenlan/workspaces/cloud-neutral-toolkit/knowledge"
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
