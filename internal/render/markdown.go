package render

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

type TOCItem struct {
	Level  int
	Title  string
	Anchor string
}

var (
	linkRe       = regexp.MustCompile(`\[(.*?)\]\((.*?)\)`)
	formattingRe = regexp.MustCompile("[`*_>#]")
)

var markdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(html.WithUnsafe()),
)

func RenderMarkdown(source string) (string, []TOCItem, string, string, error) {
	var buf bytes.Buffer
	if err := markdown.Convert([]byte(source), &buf); err != nil {
		return "", nil, "", "", err
	}

	toc := ExtractTOC(source)
	title := ExtractTitle(source)
	excerpt := ExtractExcerpt(source)
	return buf.String(), toc, title, excerpt, nil
}

func ExtractTOC(source string) []TOCItem {
	lines := strings.Split(source, "\n")
	items := make([]TOCItem, 0)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		level := 0
		for level < len(trimmed) && trimmed[level] == '#' {
			level++
		}
		if level == 0 || level > 3 {
			continue
		}
		title := strings.TrimSpace(trimmed[level:])
		if title == "" {
			continue
		}
		items = append(items, TOCItem{
			Level:  level,
			Title:  title,
			Anchor: slugify(title),
		})
	}
	return items
}

func ExtractTitle(source string) string {
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}

func ExtractExcerpt(source string) string {
	imageRe := regexp.MustCompile(`!\[.*?\]\(.*?\)`)
	for _, block := range strings.Split(source, "\n\n") {
		trimmed := strings.TrimSpace(block)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		
		// Remove images first so their alt text doesn't end up in the excerpt
		plain := imageRe.ReplaceAllString(trimmed, "")
		plain = formattingRe.ReplaceAllString(linkRe.ReplaceAllString(plain, "$1"), "")
		plain = strings.Join(strings.Fields(plain), " ")
		if plain != "" {
			if len(plain) > 220 {
				return strings.TrimSpace(plain[:220]) + "..."
			}
			return plain
		}
	}
	return ""
}

func ToPlaintext(source string) string {
	plain := formattingRe.ReplaceAllString(linkRe.ReplaceAllString(source, "$1"), "")
	return strings.Join(strings.Fields(plain), " ")
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(
		" ", "-",
		"/", "-",
		"_", "-",
		".", "-",
		"(", "",
		")", "",
		"[", "",
		"]", "",
		"'", "",
		"\"", "",
		":", "",
		",", "",
	)
	value = replacer.Replace(value)
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return strings.Trim(value, "-")
}
