package content

type TOCItem struct {
	Level  int    `json:"level"`
	Title  string `json:"title"`
	Anchor string `json:"anchor"`
}

type DocsHome struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	HTML        string `json:"html"`
}

type DocsNavigation struct {
	Title       string           `json:"title" yaml:"title"`
	Description string           `json:"description" yaml:"description"`
	Sections    []DocsNavSection `json:"sections" yaml:"sections"`
}

type DocsNavSection struct {
	Title string        `json:"title" yaml:"title"`
	Items []DocsNavItem `json:"items" yaml:"items"`
}

type DocsNavItem struct {
	Title string `json:"title" yaml:"title"`
	Href  string `json:"href" yaml:"href"`
}

type DocVersion struct {
	Slug        string    `json:"slug"`
	Label       string    `json:"label"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Language    string    `json:"language,omitempty"`
	UpdatedAt   string    `json:"updatedAt,omitempty"`
	Tags        []string  `json:"tags"`
	Markdown    string    `json:"markdown,omitempty"`
	Plaintext   string    `json:"plaintext,omitempty"`
	SourcePath  string    `json:"sourcePath,omitempty"`
	EditURL     string    `json:"editUrl,omitempty"`
	HTML        string    `json:"html"`
	TOC         []TOCItem `json:"toc"`
	Category    string    `json:"category,omitempty"`
}

type DocCollection struct {
	Slug               string       `json:"slug"`
	Title              string       `json:"title"`
	Description        string       `json:"description"`
	UpdatedAt          string       `json:"updatedAt,omitempty"`
	Tags               []string     `json:"tags"`
	Versions           []DocVersion `json:"versions"`
	DefaultVersionSlug string       `json:"defaultVersionSlug"`
}

type DocPage struct {
	Collection  DocCollection `json:"collection"`
	Version     DocVersion    `json:"version"`
	Breadcrumbs []Crumb       `json:"breadcrumbs"`
}

type Crumb struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

type BlogCategory struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type BlogPost struct {
	Slug       string        `json:"slug"`
	Title      string        `json:"title"`
	Author     string        `json:"author,omitempty"`
	Date       string        `json:"date,omitempty"`
	Tags       []string      `json:"tags"`
	Excerpt    string        `json:"excerpt"`
	HTML       string        `json:"html"`
	TOC        []TOCItem     `json:"toc"`
	Category   *BlogCategory `json:"category,omitempty"`
	Language   string        `json:"language,omitempty"`
	SourcePath string        `json:"sourcePath"`
	Plaintext  string        `json:"plaintext,omitempty"`
}

type BlogList struct {
	Posts      []BlogPost     `json:"posts"`
	Categories []BlogCategory `json:"categories"`
	Page       int            `json:"page"`
	PageSize   int            `json:"pageSize"`
	Total      int            `json:"total"`
	TotalPages int            `json:"totalPages"`
}

type SearchHit struct {
	Kind       string `json:"kind"`
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	Excerpt    string `json:"excerpt"`
	SourcePath string `json:"sourcePath"`
	HTML       string `json:"html,omitempty"`
	Plaintext  string `json:"plaintext,omitempty"`
	Collection string `json:"collection,omitempty"`
	Href       string `json:"href,omitempty"`
}

type Snapshot struct {
	DocsHomeByLang       map[string]DocsHome
	DocsNavigationByLang map[string]DocsNavigation
	Collections          []DocCollection
	CollectionsBySlug    map[string]DocCollection
	PagesByKey           map[string]DocPage
	Blogs                []BlogPost
	BlogsBySlug          map[string]BlogPost
	BlogCategories       []BlogCategory
	SourceHashes         map[string]string `json:"sourceHashes,omitempty"`
	ContentHash          string            `json:"contentHash,omitempty"`
}

type ReloadResult struct {
	Pulled   bool   `json:"pulled"`
	Reloaded bool   `json:"reloaded"`
	Message  string `json:"message,omitempty"`
	LoadedAt string `json:"loadedAt"`
}

type UpdatePlan struct {
	Kind         string   `json:"kind"`
	TargetPath   string   `json:"targetPath"`
	Allowed      bool     `json:"allowed"`
	Warnings     []string `json:"warnings"`
	Summary      string   `json:"summary"`
	DiffPreview  string   `json:"diffPreview"`
	CurrentTitle string   `json:"currentTitle,omitempty"`
	NextTitle    string   `json:"nextTitle,omitempty"`
}

type ApplyResult struct {
	TargetPath string       `json:"targetPath"`
	Bytes      int          `json:"bytes"`
	Reload     ReloadResult `json:"reload"`
}
