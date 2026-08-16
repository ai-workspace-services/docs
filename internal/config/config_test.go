package config

import (
	"testing"
)

func TestLoadSupabaseConnectURIUsesCanonicalVariable(t *testing.T) {
	t.Setenv("KNOWLEDGE_REPO_PATH", t.TempDir())
	t.Setenv("SUPABASE_CONNECT_URI", "postgres://supabase.example/content")
	t.Setenv("SUPABASE_CONNECT_URL", "postgres://legacy-alias.example/content")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got, want := cfg.SupabaseConnectURI, "postgres://supabase.example/content"; got != want {
		t.Fatalf("SupabaseConnectURI = %q, want %q", got, want)
	}
}

func TestLoadSupabaseConnectURLIsTransitionAlias(t *testing.T) {
	t.Setenv("KNOWLEDGE_REPO_PATH", t.TempDir())
	t.Setenv("SUPABASE_CONNECT_URI", "")
	t.Setenv("SUPABASE_CONNECT_URL", "postgres://supabase-alias.example/content")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got, want := cfg.SupabaseConnectURI, "postgres://supabase-alias.example/content"; got != want {
		t.Fatalf("SupabaseConnectURI = %q, want %q", got, want)
	}
}

func TestCloudRunPortOverridesVPSDefault(t *testing.T) {
	t.Setenv("KNOWLEDGE_REPO_PATH", t.TempDir())
	t.Setenv("DOCS_SERVICE_PORT", "")
	t.Setenv("PORT", "8080")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load port-aware config: %v", err)
	}
	if got, want := cfg.Port, "8080"; got != want {
		t.Fatalf("Port = %q, want %q", got, want)
	}
}

func TestVPSDefaultPortIsPreserved(t *testing.T) {
	t.Setenv("KNOWLEDGE_REPO_PATH", t.TempDir())
	t.Setenv("DOCS_SERVICE_PORT", "")
	t.Setenv("PORT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load VPS config: %v", err)
	}
	if got, want := cfg.Port, "8084"; got != want {
		t.Fatalf("Port = %q, want %q", got, want)
	}
}
