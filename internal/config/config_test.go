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
