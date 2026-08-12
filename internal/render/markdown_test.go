package render

import (
	"strings"
	"testing"
)

func TestRenderMarkdownKeepsMermaidCodeBlockForClientRendering(t *testing.T) {
	html, _, _, _, err := RenderMarkdown("```mermaid\nflowchart LR\n  A --> B\n```")
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	if !strings.Contains(html, `class="language-mermaid"`) {
		t.Fatalf("expected Mermaid language class in rendered HTML, got %q", html)
	}
}
