package content

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"docs.svc.plus/internal/render"
	"gopkg.in/yaml.v3"
)

type Indexer struct {
	RepoPath string
}

func NewIndexer(repoPath string) *Indexer {
	return &Indexer{RepoPath: repoPath}
}

func (i *Indexer) Build() (*Snapshot, error) {
	docsNavigation, err := i.buildDocsNavigation()
	if err != nil {
		return nil, err
	}
	collections, pagesByKey, err := i.buildDocs()
	if err != nil {
		return nil, err
	}
	blogs, blogMap, categories, err := i.buildBlogs()
	if err != nil {
		return nil, err
	}
	products, productMap, err := i.buildWebsiteProducts()
	if err != nil {
		return nil, err
	}
	homepageByLang := i.buildWebsiteHomepage()
	sourceHashes, contentHash, err := i.buildSourceHashes()
	if err != nil {
		return nil, err
	}

	return &Snapshot{
		DocsHomeByLang:            i.buildDocsHome(),
		DocsNavigationByLang:      docsNavigation,
		Collections:               collections,
		CollectionsBySlug:         mapCollections(collections),
		PagesByKey:                pagesByKey,
		Blogs:                     blogs,
		BlogsBySlug:               blogMap,
		BlogCategories:            categories,
		WebsiteProducts:           products,
		WebsiteProductsBySlugLang: productMap,
		WebsiteHomepageByLang:     homepageByLang,
		SourceHashes:              sourceHashes,
		ContentHash:               contentHash,
	}, nil
}

// buildSourceHashes creates a deterministic content fingerprint without storing
// content outside the in-memory snapshot. It is used for observability,
// conditional reloads, and proving that an unchanged content revision remains
// unchanged across reloads.
func (i *Indexer) buildSourceHashes() (map[string]string, string, error) {
	hashes := make(map[string]string)
	for _, rootName := range []string{"docs", "content"} {
		root := filepath.Join(i.RepoPath, rootName)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, "", fmt.Errorf("stat %s: %w", rootName, err)
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || !isHashableSource(path) {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(i.RepoPath, path)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(raw)
			hashes[filepath.ToSlash(relative)] = hex.EncodeToString(digest[:])
			return nil
		})
		if err != nil {
			return nil, "", fmt.Errorf("hash %s: %w", rootName, err)
		}
	}

	paths := make([]string, 0, len(hashes))
	for path := range hashes {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	combined := sha256.New()
	for _, path := range paths {
		_, _ = combined.Write([]byte(path))
		_, _ = combined.Write([]byte{0})
		_, _ = combined.Write([]byte(hashes[path]))
		_, _ = combined.Write([]byte{0})
	}
	return hashes, hex.EncodeToString(combined.Sum(nil)), nil
}

func isHashableSource(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".mdx" || ext == ".yaml" || ext == ".yml"
}

func (i *Indexer) buildDocsNavigation() (map[string]DocsNavigation, error) {
	result := map[string]DocsNavigation{}
	files := map[string]string{
		"zh":      filepath.Join(i.RepoPath, "docs", "navigation.zh.yaml"),
		"en":      filepath.Join(i.RepoPath, "docs", "navigation.en.yaml"),
		"default": filepath.Join(i.RepoPath, "docs", "navigation.yaml"),
	}
	for lang, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		var nav DocsNavigation
		if err := yaml.Unmarshal(raw, &nav); err != nil {
			return nil, err
		}
		result[lang] = nav
	}
	return result, nil
}

func (i *Indexer) buildDocsHome() map[string]DocsHome {
	result := map[string]DocsHome{}
	files := map[string]string{
		"zh":      filepath.Join(i.RepoPath, "docs", "zh", "README.md"),
		"en":      filepath.Join(i.RepoPath, "docs", "en", "README.md"),
		"default": filepath.Join(i.RepoPath, "docs", "index.md"),
	}
	for lang, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		meta, body := parseFrontmatter(string(content))
		html, _, title, excerpt, renderErr := render.RenderMarkdown(body)
		if renderErr != nil {
			continue
		}
		result[lang] = DocsHome{
			Title:       pickString(meta, "title", title, "Documentation"),
			Description: pickString(meta, "description", excerpt, ""),
			HTML:        html,
		}
	}
	if _, ok := result["default"]; !ok {
		result["default"] = DocsHome{Title: "Documentation"}
	}
	return result
}

func (i *Indexer) buildDocs() ([]DocCollection, map[string]DocPage, error) {
	root := filepath.Join(i.RepoPath, "docs")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return []DocCollection{}, map[string]DocPage{}, nil
	} else if err != nil {
		return nil, nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, fmt.Errorf("read docs root: %w", err)
	}
	collections := make([]DocCollection, 0)
	pages := make(map[string]DocPage)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "zh" || name == "en" || strings.HasPrefix(name, ".") {
			continue
		}
		collectionPath := filepath.Join(root, name)
		versions := make([]DocVersion, 0)
		err = filepath.WalkDir(collectionPath, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if !isMarkdown(path) {
				return nil
			}
			version, page, buildErr := buildDocVersion(root, name, path)
			if buildErr != nil {
				return buildErr
			}
			versions = append(versions, version)
			if version.Language != "" {
				pages[pageKeyWithLang(name, version.Language, version.Slug)] = page
				if _, exists := pages[pageKey(name, version.Slug)]; !exists {
					pages[pageKey(name, version.Slug)] = page
				}
			} else {
				pages[pageKey(name, version.Slug)] = page
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
		if len(versions) == 0 {
			continue
		}
		sortDocVersions(versions)
		primary := versions[0]
		defaultSlug := primary.Slug
		for _, version := range versions {
			if version.Slug == "overview" {
				defaultSlug = version.Slug
				primary = version
				break
			}
		}
		collections = append(collections, DocCollection{
			Slug:               name,
			Title:              humanize(strings.TrimPrefix(name, "0")),
			Description:        primary.Description,
			UpdatedAt:          primary.UpdatedAt,
			Tags:               collectTags(versions),
			Versions:           versions,
			DefaultVersionSlug: defaultSlug,
		})
	}
	slices.SortFunc(collections, func(a, b DocCollection) int {
		return strings.Compare(a.Slug, b.Slug)
	})
	for idx := range collections {
		collection := collections[idx]
		for _, version := range collection.Versions {
			page := pages[pageKey(collection.Slug, version.Slug)]
			page.Collection = collection
			pages[pageKey(collection.Slug, version.Slug)] = page
		}
	}
	return collections, pages, nil
}

func (i *Indexer) buildBlogs() ([]BlogPost, map[string]BlogPost, []BlogCategory, error) {
	root := filepath.Join(i.RepoPath, "content")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return []BlogPost{}, map[string]BlogPost{}, []BlogCategory{}, nil
	} else if err != nil {
		return nil, nil, nil, err
	}
	posts := make([]BlogPost, 0)
	postMap := make(map[string]BlogPost)
	categoryMap := make(map[string]BlogCategory)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !isMarkdown(path) {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		relSlash := filepath.ToSlash(relative)
		if strings.HasPrefix(relSlash, "website/") || relSlash == "website" {
			return nil
		}
		post, err := buildBlogPost(root, path)
		if err != nil {
			return err
		}
		posts = append(posts, post)
		postMap[post.Slug] = post
		if post.Category != nil {
			categoryMap[post.Category.Key] = *post.Category
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	slices.SortFunc(posts, func(a, b BlogPost) int {
		return strings.Compare(b.Date, a.Date)
	})
	categories := make([]BlogCategory, 0, len(categoryMap))
	for _, category := range categoryMap {
		categories = append(categories, category)
	}
	slices.SortFunc(categories, func(a, b BlogCategory) int {
		return strings.Compare(a.Key, b.Key)
	})
	return posts, postMap, categories, nil
}

type rawProductFrontmatter struct {
	Hero      WebsiteHero       `yaml:"hero"`
	Wizard    *WebsiteWizard    `yaml:"wizard,omitempty"`
	Showcases []WebsiteShowcase `yaml:"showcases"`
}

func (i *Indexer) buildWebsiteProducts() ([]WebsiteProduct, map[string]WebsiteProduct, error) {
	root := filepath.Join(i.RepoPath, "content", "website", "product")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return []WebsiteProduct{}, map[string]WebsiteProduct{}, nil
	} else if err != nil {
		return nil, nil, err
	}

	products := make([]WebsiteProduct, 0)
	productMap := make(map[string]WebsiteProduct)

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, fmt.Errorf("read website product root: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		slug := entry.Name()
		productDir := filepath.Join(root, slug)

		_ = filepath.WalkDir(productDir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() || !isMarkdown(path) {
				return nil
			}

			raw, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			stat, _ := os.Stat(path)

			frontmatterBytes, _ := extractFrontmatterRaw(string(raw))
			var rawData rawProductFrontmatter
			if len(frontmatterBytes) > 0 {
				_ = yaml.Unmarshal(frontmatterBytes, &rawData)
			}

			if strings.TrimSpace(rawData.Hero.Title) == "" {
				rawData.Hero.Title = humanize(slug)
			}
			if strings.TrimSpace(rawData.Hero.CTA.Href) == "" {
				rawData.Hero.CTA.Href = "/download"
			}
			if strings.TrimSpace(rawData.Hero.CTA.Label) == "" {
				rawData.Hero.CTA.Label = "Download"
			}

			rel, _ := filepath.Rel(productDir, path)
			parts := strings.Split(filepath.ToSlash(rel), "/")
			lang := "zh"
			if len(parts) >= 2 && (parts[0] == "zh" || parts[0] == "en") {
				lang = parts[0]
			}

			var updatedAt string
			if stat != nil {
				updatedAt = stat.ModTime().UTC().Format(time.RFC3339)
			}

			sourcePath, _ := filepath.Rel(i.RepoPath, path)

			prod := WebsiteProduct{
				Slug:       slug,
				Language:   lang,
				Hero:       rawData.Hero,
				Wizard:     rawData.Wizard,
				Showcases:  rawData.Showcases,
				SourcePath: filepath.ToSlash(sourcePath),
				UpdatedAt:  updatedAt,
			}

			products = append(products, prod)
			productMap[slug+":"+lang] = prod
			if lang == "zh" {
				if _, exists := productMap[slug]; !exists {
					productMap[slug] = prod
				}
			}
			return nil
		})
	}

	slices.SortFunc(products, func(a, b WebsiteProduct) int {
		return strings.Compare(a.Slug, b.Slug)
	})

	return products, productMap, nil
}

func (i *Indexer) buildWebsiteHomepage() map[string]map[string]any {
	result := map[string]map[string]any{}
	files := map[string]string{
		"zh":      filepath.Join(i.RepoPath, "content", "website", "homepage", "zh", "marketing.md"),
		"en":      filepath.Join(i.RepoPath, "content", "website", "homepage", "en", "marketing.md"),
		"default": filepath.Join(i.RepoPath, "content", "website", "homepage", "zh", "marketing.md"),
	}
	for lang, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		meta, _ := parseFrontmatter(string(raw))
		if len(meta) > 0 {
			result[lang] = meta
		}
	}
	return result
}

func buildDocVersion(docsRoot, collection, absolutePath string) (DocVersion, DocPage, error) {
	raw, err := os.ReadFile(absolutePath)
	if err != nil {
		return DocVersion{}, DocPage{}, err
	}
	stat, err := os.Stat(absolutePath)
	if err != nil {
		return DocVersion{}, DocPage{}, err
	}
	meta, body := parseFrontmatter(string(raw))
	html, toc, title, excerpt, err := render.RenderMarkdown(body)
	if err != nil {
		return DocVersion{}, DocPage{}, err
	}
	relative, _ := filepath.Rel(filepath.Join(docsRoot, collection), absolutePath)
	slug := pickString(meta, "slug", docSlug(relative), "overview")
	language := pickString(meta, "lang", "", "")
	version := DocVersion{
		Slug:        slug,
		Label:       pickString(meta, "version", filepath.Base(slug), "latest"),
		Title:       pickString(meta, "title", title, humanize(filepath.Base(slug))),
		Description: pickString(meta, "description", excerpt, ""),
		Language:    language,
		UpdatedAt:   stat.ModTime().UTC().Format(time.RFC3339),
		Tags:        stringSlice(meta["tags"]),
		Markdown:    body,
		Plaintext:   render.ToPlaintext(body),
		SourcePath:  filepath.ToSlash(filepath.Join("docs", collection, relative)),
		HTML:        html,
		TOC:         mapTOC(toc),
		Category:    pickString(meta, "category", "", ""),
	}
	page := DocPage{
		Version: version,
		Breadcrumbs: []Crumb{
			{Label: "Documentation", Href: "/docs"},
			{Label: humanize(strings.TrimPrefix(collection, "0")), Href: "/docs/" + collection},
			{Label: version.Title, Href: "/docs/" + collection + "/" + slug},
		},
	}
	return version, page, nil
}

func buildBlogPost(contentRoot, absolutePath string) (BlogPost, error) {
	raw, err := os.ReadFile(absolutePath)
	if err != nil {
		return BlogPost{}, err
	}
	stat, err := os.Stat(absolutePath)
	if err != nil {
		return BlogPost{}, err
	}
	meta, body := parseFrontmatter(string(raw))
	html, toc, title, excerpt, err := render.RenderMarkdown(body)
	if err != nil {
		return BlogPost{}, err
	}
	relative, _ := filepath.Rel(contentRoot, absolutePath)
	slug := strings.TrimSuffix(filepath.ToSlash(relative), filepath.Ext(relative))
	category := resolveBlogCategory(strings.Split(slug, "/"))
	date := pickString(meta, "date", stat.ModTime().UTC().Format(time.RFC3339), "")
	author := pickString(meta, "author", "", "")
	lang := "en"
	if containsCJK(title + " " + body) {
		lang = "zh"
	}
	return BlogPost{
		Slug:       slug,
		Title:      pickString(meta, "title", title, humanize(filepath.Base(slug))),
		Author:     author,
		Date:       date,
		Tags:       stringSlice(meta["tags"]),
		Excerpt:    pickString(meta, "excerpt", excerpt, ""),
		HTML:       html,
		TOC:        mapTOC(toc),
		Category:   category,
		Language:   lang,
		SourcePath: filepath.ToSlash(filepath.Join("content", relative)),
		Plaintext:  render.ToPlaintext(body),
	}, nil
}

func extractFrontmatterRaw(raw string) ([]byte, string) {
	if !strings.HasPrefix(raw, "---\n") {
		return nil, raw
	}
	rest := strings.TrimPrefix(raw, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return nil, raw
	}
	return []byte(rest[:idx]), rest[idx+5:]
}

func parseFrontmatter(raw string) (map[string]any, string) {
	frontmatterBytes, body := extractFrontmatterRaw(raw)
	if len(frontmatterBytes) == 0 {
		return map[string]any{}, raw
	}
	meta := map[string]any{}
	if err := yaml.Unmarshal(frontmatterBytes, &meta); err != nil {
		return map[string]any{}, raw
	}
	return meta, body
}

func pickString(meta map[string]any, key string, fallback string, defaultValue string) string {
	if raw, ok := meta[key]; ok {
		if value, ok := raw.(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if strings.TrimSpace(fallback) != "" {
		return strings.TrimSpace(fallback)
	}
	return defaultValue
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				out = append(out, strings.TrimSpace(str))
			}
		}
		return out
	case []string:
		return typed
	default:
		return []string{}
	}
}

func docSlug(relative string) string {
	slug := strings.TrimSuffix(filepath.ToSlash(relative), filepath.Ext(relative))
	slug = strings.TrimSuffix(slug, "/README")
	slug = strings.TrimSuffix(slug, "/index")
	if slug == "README" || slug == "index" || slug == "" {
		return "overview"
	}
	return slug
}

func humanize(value string) string {
	value = strings.Trim(value, "-_/")
	value = regexp.MustCompile(`^\d+[-_]*`).ReplaceAllString(value, "")
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '-' || r == '_' || r == '/'
	})
	for i := range parts {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, " ")
}

func isMarkdown(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".mdx"
}

func sortDocVersions(versions []DocVersion) {
	slices.SortFunc(versions, func(a, b DocVersion) int {
		if a.Slug == "overview" {
			return -1
		}
		if b.Slug == "overview" {
			return 1
		}
		return strings.Compare(a.Slug, b.Slug)
	})
}

func collectTags(versions []DocVersion) []string {
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

func mapCollections(collections []DocCollection) map[string]DocCollection {
	out := make(map[string]DocCollection, len(collections))
	for _, collection := range collections {
		out[collection.Slug] = collection
	}
	return out
}

func pageKey(collection, slug string) string {
	return collection + "::" + slug
}

func pageKeyWithLang(collection, lang, slug string) string {
	return collection + ":" + lang + "::" + slug
}

func mapTOC(items []render.TOCItem) []TOCItem {
	out := make([]TOCItem, 0, len(items))
	for _, item := range items {
		out = append(out, TOCItem{Level: item.Level, Title: item.Title, Anchor: item.Anchor})
	}
	return out
}

func containsCJK(value string) bool {
	for _, r := range value {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

func resolveBlogCategory(segments []string) *BlogCategory {
	if len(segments) == 0 {
		return nil
	}
	switch segments[0] {
	case "04-infra-platform":
		return &BlogCategory{Key: "infra-cloud", Label: "Infra & Cloud"}
	case "03-observability":
		return &BlogCategory{Key: "observability", Label: "Observability"}
	case "01-id-security":
		return &BlogCategory{Key: "identity", Label: "ID & Security"}
	case "02-iac-devops":
		return &BlogCategory{Key: "iac-devops", Label: "IaC & DevOps"}
	case "05-data-ai":
		return &BlogCategory{Key: "data-ai", Label: "Data & AI"}
	case "06-workshops":
		return &BlogCategory{Key: "workshops", Label: "Workshops"}
	case "00-global":
		if len(segments) > 1 && segments[1] == "essays" {
			return &BlogCategory{Key: "essays", Label: "随笔&观察"}
		}
		return &BlogCategory{Key: "insight", Label: "资讯"}
	default:
		return nil
	}
}
