package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"docs.svc.plus/internal/agent"
	"docs.svc.plus/internal/config"
	"docs.svc.plus/internal/content"
	gitsync "docs.svc.plus/internal/git"
)

type App struct {
	cfg     config.Config
	indexer *content.Indexer
	agent   *agent.Handler

	mu       sync.RWMutex
	snapshot *content.Snapshot
	loadedAt time.Time
}

func NewApp(cfg config.Config) (*App, error) {
	indexer := content.NewIndexer(cfg.KnowledgeRepoPath)
	snapshot, err := indexer.Build()
	if err != nil {
		return nil, err
	}
	app := &App{
		cfg:      cfg,
		indexer:  indexer,
		snapshot: snapshot,
		loadedAt: time.Now().UTC(),
	}
	app.agent = agent.NewHandler(app)
	return app, nil
}

func (a *App) RepoPath() string {
	return a.cfg.KnowledgeRepoPath
}

func (a *App) GetSnapshot() *content.Snapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.snapshot
}

func (a *App) Reload(pull bool) content.ReloadResult {
	result := content.ReloadResult{
		Pulled:   false,
		Reloaded: false,
		LoadedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if pull {
		ok, message, err := gitsync.Sync(a.cfg.KnowledgeRepoURL, a.cfg.KnowledgeRepoPath)
		result.Pulled = ok
		result.Message = message
		if err != nil {
			result.Message = err.Error()
			return result
		}
	}
	snapshot, err := a.indexer.Build()
	if err != nil {
		result.Message = err.Error()
		return result
	}
	a.mu.Lock()
	a.snapshot = snapshot
	a.loadedAt = time.Now().UTC()
	a.mu.Unlock()
	result.Reloaded = true
	return result
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.handleHealth)
	mux.HandleFunc("/docs", a.handlePublicDocs)
	mux.HandleFunc("/docs/", a.handlePublicDocs)

	api := http.NewServeMux()
	api.HandleFunc("/api/v1/docs/home", a.handleDocsHome)
	api.HandleFunc("/api/v1/docs/collections", a.handleDocCollections)
	api.HandleFunc("/api/v1/docs/search", a.handleDocsSearch)
	api.HandleFunc("/api/v1/docs/pages/", a.handleDocPage)
	api.HandleFunc("/api/v1/blogs/", a.handleBlogPost)
	api.HandleFunc("/api/v1/blogs", a.handleBlogs)
	api.HandleFunc("/api/v1/home/latest-blogs", a.handleLatestBlogs)
	api.HandleFunc("/api/v1/admin/reload", a.handleReload)
	api.HandleFunc("/api/v1/agent/invoke", a.handleAgentInvoke)

	mux.Handle("/api/", RequireServiceToken(a.cfg.InternalServiceToken, api))
	return mux
}

func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"loadedAt": a.loadedAt.Format(time.RFC3339),
	})
}

func (a *App) handleDocsHome(w http.ResponseWriter, r *http.Request) {
	lang := resolveLang(r)
	snapshot := a.GetSnapshot()
	home, ok := snapshot.DocsHomeByLang[lang]
	if !ok {
		home = snapshot.DocsHomeByLang["default"]
	}
	writeJSON(w, http.StatusOK, home)
}

func (a *App) handleDocCollections(w http.ResponseWriter, r *http.Request) {
	lang := resolveLang(r)
	snapshot := a.GetSnapshot()
	// Filter collections by language if needed
	collections := snapshot.Collections
	if lang != "default" {
		// Return collections with language-specific metadata
		collections = filterCollectionsByLang(collections, lang)
	}
	writeJSON(w, http.StatusOK, collections)
}

func (a *App) handleDocPage(w http.ResponseWriter, r *http.Request) {
	lang := resolveLang(r)
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/docs/pages/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "collection_and_slug_required"})
		return
	}

	// Try language-specific key first, then fallback
	pageKey := parts[0] + "::" + parts[1]
	snapshot := a.GetSnapshot()

	// If language is specified, try to find language-specific version
	if lang != "default" {
		langKey := parts[0] + ":" + lang + "::" + parts[1]
		if page, ok := snapshot.PagesByKey[langKey]; ok {
			writeJSON(w, http.StatusOK, a.withRepositoryLinks(page))
			return
		}
	}

	page, ok := snapshot.PagesByKey[pageKey]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, a.withRepositoryLinks(page))
}

func (a *App) handleDocsSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if query == "" {
		writeJSON(w, http.StatusOK, []content.SearchHit{})
		return
	}

	limit := parseInt(r.URL.Query().Get("limit"), 10)
	if limit > 20 {
		limit = 20
	}
	terms := strings.Fields(strings.ToLower(query))
	collections := filterCollectionsByLang(a.GetSnapshot().Collections, resolveLang(r))
	type rankedHit struct {
		hit   content.SearchHit
		score int
	}
	ranked := make([]rankedHit, 0)
	for _, collection := range collections {
		for _, version := range collection.Versions {
			title := strings.ToLower(version.Title)
			description := strings.ToLower(version.Description)
			haystack := strings.ToLower(strings.Join([]string{
				version.Title,
				version.Description,
				version.Plaintext,
				strings.Join(version.Tags, " "),
			}, " "))
			score := 0
			matched := true
			for _, term := range terms {
				if !strings.Contains(haystack, term) {
					matched = false
					break
				}
				score++
				if strings.Contains(title, term) {
					score += 8
				} else if strings.Contains(description, term) {
					score += 4
				}
			}
			if !matched {
				continue
			}
			href := "/docs/" + collection.Slug + "/" + version.Slug
			ranked = append(ranked, rankedHit{
				score: score,
				hit: content.SearchHit{
					Kind:       "doc",
					Slug:       collection.Slug + "/" + version.Slug,
					Title:      version.Title,
					Excerpt:    version.Description,
					SourcePath: version.SourcePath,
					Collection: collection.Title,
					Href:       href,
				},
			})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	hits := make([]content.SearchHit, 0, len(ranked))
	for _, item := range ranked {
		hits = append(hits, item.hit)
	}
	writeJSON(w, http.StatusOK, hits)
}

func (a *App) withRepositoryLinks(page content.DocPage) content.DocPage {
	page.Version.EditURL = githubEditURL(a.cfg.KnowledgeRepoURL, page.Version.SourcePath)
	return page
}

func githubEditURL(repositoryURL, sourcePath string) string {
	repositoryURL = strings.TrimSpace(repositoryURL)
	sourcePath = strings.TrimLeft(strings.TrimSpace(sourcePath), "/")
	if repositoryURL == "" || sourcePath == "" {
		return ""
	}
	if strings.HasPrefix(repositoryURL, "git@github.com:") {
		repositoryURL = "https://github.com/" + strings.TrimPrefix(repositoryURL, "git@github.com:")
	}
	parsed, err := url.Parse(strings.TrimSuffix(repositoryURL, ".git"))
	if err != nil || parsed.Hostname() != "github.com" {
		return ""
	}
	return strings.TrimRight(parsed.String(), "/") + "/edit/main/" + sourcePath
}

// filterCollectionsByLang filters collection metadata by language
func filterCollectionsByLang(collections []content.DocCollection, lang string) []content.DocCollection {
	if lang == "" || lang == "default" {
		return collections
	}

	filtered := make([]content.DocCollection, 0, len(collections))
	for _, collection := range collections {
		bySlug := make(map[string]content.DocVersion)
		order := make([]string, 0, len(collection.Versions))
		for _, version := range collection.Versions {
			if version.Language != "" && version.Language != lang {
				continue
			}
			if _, exists := bySlug[version.Slug]; !exists {
				order = append(order, version.Slug)
			}
			if existing, exists := bySlug[version.Slug]; exists && existing.Language == lang {
				continue
			}
			bySlug[version.Slug] = version
		}
		if len(bySlug) == 0 {
			filtered = append(filtered, collection)
			continue
		}
		versions := make([]content.DocVersion, 0, len(order))
		for _, slug := range order {
			versions = append(versions, bySlug[slug])
		}
		next := collection
		next.Versions = versions
		next.Tags = collectCollectionTags(versions)
		for _, version := range versions {
			if version.Slug == collection.DefaultVersionSlug {
				filtered = append(filtered, next)
				goto nextCollection
			}
		}
		next.DefaultVersionSlug = versions[0].Slug
		filtered = append(filtered, next)
	nextCollection:
	}
	return filtered
}

func collectCollectionTags(versions []content.DocVersion) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, version := range versions {
		for _, tag := range version.Tags {
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}
	return out
}

func (a *App) handleBlogs(w http.ResponseWriter, r *http.Request) {
	lang := resolveLang(r)
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	page := parseInt(r.URL.Query().Get("page"), 1)
	pageSize := parseInt(r.URL.Query().Get("pageSize"), 10)
	if pageSize > 50 {
		pageSize = 50
	}

	filtered := make([]content.BlogPost, 0)
	for _, post := range a.GetSnapshot().Blogs {
		// Filter by language if specified
		if lang != "default" && post.Language != "" && post.Language != lang {
			continue
		}
		if category != "" && (post.Category == nil || post.Category.Key != category) {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(post.Title + " " + post.Excerpt + " " + post.Plaintext)
			if !strings.Contains(haystack, strings.ToLower(query)) {
				continue
			}
		}
		filtered = append(filtered, post)
	}

	total := len(filtered)
	totalPages := 1
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, content.BlogList{
		Posts:      filtered[start:end],
		Categories: a.GetSnapshot().BlogCategories,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	})
}

func (a *App) handleBlogPost(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/api/v1/blogs/")
	post, ok := a.GetSnapshot().BlogsBySlug[slug]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, post)
}

func (a *App) handleLatestBlogs(w http.ResponseWriter, r *http.Request) {
	lang := resolveLang(r)
	limit := parseInt(r.URL.Query().Get("limit"), 7)
	if limit > 20 {
		limit = 20
	}

	posts := a.GetSnapshot().Blogs

	// Filter by language if specified
	if lang != "default" {
		filtered := make([]content.BlogPost, 0)
		for _, post := range posts {
			if post.Language == "" || post.Language == lang {
				filtered = append(filtered, post)
			}
		}
		posts = filtered
	}

	if len(posts) > limit {
		posts = posts[:limit]
	}
	writeJSON(w, http.StatusOK, posts)
}

func (a *App) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	result := a.Reload(r.URL.Query().Get("pull") != "false")
	status := http.StatusOK
	if !result.Reloaded {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, result)
}

func (a *App) handleAgentInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	var req agent.InvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	result, err := a.agent.Invoke(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tool": req.Tool, "result": result})
}

func resolveLang(r *http.Request) string {
	// 1. URL query parameter takes highest priority
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))
	if lang == "zh" || lang == "en" {
		return lang
	}

	// 2. Check X-Language header (global state from console)
	lang = strings.TrimSpace(r.Header.Get("X-Language"))
	if lang == "zh" || lang == "en" {
		return lang
	}

	// 3. Check Accept-Language header
	acceptLang := r.Header.Get("Accept-Language")
	if strings.Contains(strings.ToLower(acceptLang), "zh") {
		return "zh"
	}
	if strings.Contains(strings.ToLower(acceptLang), "en") {
		return "en"
	}

	// 4. Default fallback
	return "default"
}

func parseInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
