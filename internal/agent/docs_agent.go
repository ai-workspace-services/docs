package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"docs.svc.plus/internal/content"
)

type Service interface {
	GetSnapshot() *content.Snapshot
	Reload(pull bool) content.ReloadResult
	RepoPath() string
}

type Handler struct {
	service Service
	writeMu sync.Mutex
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

type InvokeRequest struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

func (h *Handler) Invoke(req InvokeRequest) (any, error) {
	switch req.Tool {
	case "docs.search":
		return h.searchDocs(asString(req.Args["query"])), nil
	case "docs.read_page":
		return h.readPage(asString(req.Args["collection"]), asString(req.Args["slug"])), nil
	case "docs.list_collections":
		return h.service.GetSnapshot().Collections, nil
	case "blogs.search":
		return h.searchBlogs(asString(req.Args["query"])), nil
	case "blogs.read_post":
		return h.readBlog(asString(req.Args["slug"])), nil
	case "docs.plan_update":
		return h.planUpdate(req.Args)
	case "docs.apply_update":
		return h.applyUpdate(req.Args)
	case "docs.reload":
		return h.service.Reload(boolArg(req.Args["pull"])), nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", req.Tool)
	}
}

func (h *Handler) searchDocs(query string) []content.SearchHit {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return []content.SearchHit{}
	}
	out := make([]content.SearchHit, 0)
	for _, collection := range h.service.GetSnapshot().Collections {
		for _, version := range collection.Versions {
			haystack := strings.ToLower(strings.Join([]string{
				version.Title,
				version.Description,
				version.Plaintext,
				strings.Join(version.Tags, " "),
			}, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
			out = append(out, content.SearchHit{
				Kind:       "doc",
				Slug:       collection.Slug + "/" + version.Slug,
				Title:      version.Title,
				Excerpt:    version.Description,
				SourcePath: version.SourcePath,
				HTML:       version.HTML,
				Plaintext:  version.Plaintext,
				Collection: collection.Title,
				Href:       "/docs/" + collection.Slug + "/" + version.Slug,
			})
			if len(out) >= 10 {
				return out
			}
		}
	}
	return out
}

func (h *Handler) readPage(collection, slug string) any {
	page, ok := h.service.GetSnapshot().PagesByKey[collection+"::"+slug]
	if !ok {
		return map[string]any{"found": false}
	}
	return map[string]any{"found": true, "page": page}
}

func (h *Handler) searchBlogs(query string) []content.SearchHit {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return []content.SearchHit{}
	}
	out := make([]content.SearchHit, 0)
	for _, post := range h.service.GetSnapshot().Blogs {
		haystack := strings.ToLower(post.Title + " " + post.Excerpt + " " + post.Plaintext)
		if !strings.Contains(haystack, query) {
			continue
		}
		out = append(out, content.SearchHit{
			Kind:       "blog",
			Slug:       post.Slug,
			Title:      post.Title,
			Excerpt:    post.Excerpt,
			SourcePath: post.SourcePath,
			HTML:       post.HTML,
			Plaintext:  post.Plaintext,
		})
		if len(out) >= 10 {
			return out
		}
	}
	return out
}

func (h *Handler) readBlog(slug string) any {
	post, ok := h.service.GetSnapshot().BlogsBySlug[slug]
	if !ok {
		return map[string]any{"found": false}
	}
	return map[string]any{"found": true, "post": post}
}

func (h *Handler) planUpdate(args map[string]any) (content.UpdatePlan, error) {
	targetPath := deriveTargetPath(args)
	absolutePath, err := ValidateTargetPath(h.service.RepoPath(), targetPath)
	if err != nil {
		return content.UpdatePlan{
			Kind:       asString(args["kind"]),
			TargetPath: targetPath,
			Allowed:    false,
			Warnings:   []string{err.Error()},
			Summary:    "Write blocked by allowlist.",
		}, nil
	}

	currentBytes, _ := osReadFileSafe(absolutePath)
	nextContent := EnsureFrontmatter(asString(args["content"]), asString(args["title"]))
	return content.UpdatePlan{
		Kind:         asString(args["kind"]),
		TargetPath:   filepath.ToSlash(targetPath),
		Allowed:      true,
		Warnings:     []string{},
		Summary:      asString(args["summary"]),
		DiffPreview:  simpleDiff(string(currentBytes), nextContent),
		CurrentTitle: firstHeading(string(currentBytes)),
		NextTitle:    firstHeading(nextContent),
	}, nil
}

func (h *Handler) applyUpdate(args map[string]any) (content.ApplyResult, error) {
	h.writeMu.Lock()
	defer h.writeMu.Unlock()

	targetPath := deriveTargetPath(args)
	absolutePath, err := ValidateTargetPath(h.service.RepoPath(), targetPath)
	if err != nil {
		return content.ApplyResult{}, err
	}
	nextContent := EnsureFrontmatter(asString(args["content"]), asString(args["title"]))
	bytes, err := WriteFile(absolutePath, nextContent)
	if err != nil {
		return content.ApplyResult{}, err
	}
	reload := h.service.Reload(false)
	return content.ApplyResult{
		TargetPath: filepath.ToSlash(targetPath),
		Bytes:      bytes,
		Reload:     reload,
	}, nil
}

func deriveTargetPath(args map[string]any) string {
	if target := asString(args["targetPath"]); target != "" {
		return target
	}
	kind := asString(args["kind"])
	slug := strings.Trim(asString(args["slug"]), "/")
	collection := strings.Trim(asString(args["collection"]), "/")
	switch kind {
	case "docs":
		if slug == "" || slug == "overview" {
			return filepath.ToSlash(filepath.Join("docs", collection, "README.md"))
		}
		return filepath.ToSlash(filepath.Join("docs", collection, slug+".md"))
	case "blog":
		if strings.HasSuffix(slug, ".md") || strings.HasSuffix(slug, ".mdx") {
			return filepath.ToSlash(filepath.Join("content", slug))
		}
		return filepath.ToSlash(filepath.Join("content", slug+".md"))
	default:
		return slug
	}
}

func asString(value any) string {
	if typed, ok := value.(string); ok {
		return strings.TrimSpace(typed)
	}
	return ""
}

func boolArg(value any) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return false
}

func osReadFileSafe(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func simpleDiff(before, after string) string {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	max := len(beforeLines)
	if len(afterLines) > max {
		max = len(afterLines)
	}
	out := []string{"--- before", "+++ after"}
	for i := 0; i < max && len(out) < 80; i++ {
		var a, b string
		if i < len(beforeLines) {
			a = beforeLines[i]
		}
		if i < len(afterLines) {
			b = afterLines[i]
		}
		if a == b {
			continue
		}
		if a != "" {
			out = append(out, "- "+a)
		}
		if b != "" {
			out = append(out, "+ "+b)
		}
	}
	return strings.Join(out, "\n")
}

func firstHeading(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimPrefix(trimmed, "# ")
		}
	}
	return ""
}
