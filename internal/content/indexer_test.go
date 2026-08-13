package content

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildCreatesStableSourceHashes(t *testing.T) {
	repo := t.TempDir()
	writeIndexerFile(t, filepath.Join(repo, "docs", "guide", "overview.md"), "---\ntitle: Guide\n---\n# Guide\n\nContent")
	writeIndexerFile(t, filepath.Join(repo, "content", "engineering", "post.md"), "---\ntitle: Post\nlang: en\n---\n# Post\n\nContent")

	first, err := NewIndexer(repo).Build()
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	second, err := NewIndexer(repo).Build()
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	if first.ContentHash == "" || first.ContentHash != second.ContentHash {
		t.Fatalf("expected stable content hash, first=%q second=%q", first.ContentHash, second.ContentHash)
	}
	if got := first.SourceHashes["docs/guide/overview.md"]; got == "" {
		t.Fatalf("expected document source hash")
	}
	if got := first.SourceHashes["content/engineering/post.md"]; got == "" {
		t.Fatalf("expected blog source hash")
	}
}

func writeIndexerFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
