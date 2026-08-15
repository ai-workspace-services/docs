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

func TestBuildWebsiteProductsAndHomepage(t *testing.T) {
	repo := t.TempDir()
	productZh := `---
hero:
  badge: "AI 连接"
  title: "XConnect"
  subtitle: "安全加速"
  cta:
    label: "下载"
    href: "/download"
  supportedPlatforms: "macOS · iOS"
wizard:
  title: "向导"
  description: "3 步开启"
  steps:
    - step: 1
      title: "获取客户端"
      description: "下载"
      platforms: "macOS"
showcases:
  - title: "全球加速"
    description: "专线直连"
    icon: "zap"
    image: "/marketing/xconnect/product.png"
---
`
	productEn := `---
hero:
  badge: "AI Connectivity"
  title: "XConnect"
  subtitle: "Secure Acceleration"
  cta:
    label: "Download"
    href: "/download"
---
`
	homepageZh := `---
brand:
  title: "XWorkmate"
hero:
  title:
    - "开放的 AI 工作空间"
---
`
	writeIndexerFile(t, filepath.Join(repo, "content", "website", "product", "xconnect", "zh", "hero.md"), productZh)
	writeIndexerFile(t, filepath.Join(repo, "content", "website", "product", "xconnect", "en", "hero.md"), productEn)
	writeIndexerFile(t, filepath.Join(repo, "content", "website", "homepage", "zh", "marketing.md"), homepageZh)

	snapshot, err := NewIndexer(repo).Build()
	if err != nil {
		t.Fatalf("build error: %v", err)
	}

	// Verify products
	if len(snapshot.WebsiteProducts) != 2 {
		t.Fatalf("expected 2 website product variants, got %d", len(snapshot.WebsiteProducts))
	}
	prodZh, ok := snapshot.WebsiteProductsBySlugLang["xconnect:zh"]
	if !ok {
		t.Fatalf("expected xconnect:zh in productMap")
	}
	if prodZh.Hero.Title != "XConnect" || prodZh.Hero.Badge != "AI 连接" {
		t.Errorf("unexpected hero title/badge: %+v", prodZh.Hero)
	}
	if prodZh.Wizard == nil || len(prodZh.Wizard.Steps) != 1 {
		t.Errorf("unexpected wizard: %+v", prodZh.Wizard)
	}
	if len(prodZh.Showcases) != 1 || prodZh.Showcases[0].Icon != "zap" {
		t.Errorf("unexpected showcases: %+v", prodZh.Showcases)
	}

	// Verify homepage
	homeZh, ok := snapshot.WebsiteHomepageByLang["zh"]
	if !ok {
		t.Fatalf("expected zh homepage marketing in snapshot")
	}
	if brand, ok := homeZh["brand"].(map[string]any); !ok || brand["title"] != "XWorkmate" {
		t.Errorf("unexpected homepage brand: %+v", homeZh["brand"])
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
