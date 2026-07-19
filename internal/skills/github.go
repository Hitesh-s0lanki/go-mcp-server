package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

// githubAPIBase is the GitHub REST root. The downloader enumerates a skill's
// files through the contents API and pulls each file's raw bytes.
const githubAPIBase = "https://api.github.com"

// Download bounds. A skill is a small directory (a SKILL.md plus a handful of
// scripts/references); these caps stop a mistaken ref pointing at a huge repo
// from turning one call into a crawl.
const (
	maxSkillFiles          = 50
	maxSkillDepth          = 3
	maxFileBytes           = 256 * 1024 // per file
	maxDownloadConcurrency = 6
)

// Downloader fetches a complete skill from GitHub: it lists the skill directory
// (recursively) and downloads every file's raw content concurrently. It is
// deterministic and needs no model -- discovery is skills_find's job; this just
// materialises a skill you already located.
type Downloader struct {
	Token  string // optional GITHUB_TOKEN; raises the 60/hr unauthenticated limit
	Client *http.Client

	apiBase string // unexported; the const in prod, overridden by tests

	// skipHostCheck relaxes the githubusercontent.com guard on raw fetches. It is
	// only ever set by tests, whose stub server is on 127.0.0.1; production always
	// leaves it false so the guard stands.
	skipHostCheck bool
}

// NewDownloader builds a downloader. An empty token is fine (public repos work
// unauthenticated, just rate-limited).
func NewDownloader(token string) *Downloader {
	return &Downloader{
		Token:   token,
		Client:  &http.Client{Timeout: 60 * time.Second},
		apiBase: githubAPIBase,
	}
}

// SkillFile is one downloaded file. Content is empty and Error set when that
// single file failed -- one bad file does not sink the whole download.
type SkillFile struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Bytes   int    `json:"bytes"`
	Error   string `json:"error,omitempty"`
}

// DownloadResult is a complete, ready-to-install skill.
type DownloadResult struct {
	Name       string      `json:"name"`
	Source     string      `json:"source"`
	Ref        string      `json:"ref,omitempty"`
	Files      []SkillFile `json:"files"`
	TotalBytes int         `json:"total_bytes"`
}

// skillRef is a parsed GitHub location.
type skillRef struct {
	owner, repo, ref, dir string
}

// parseSkillRef accepts the shapes a skill link realistically arrives in:
//
//	https://github.com/<owner>/<repo>/tree/<branch>/<path>
//	https://github.com/<owner>/<repo>/blob/<branch>/<path>/SKILL.md
//	https://raw.githubusercontent.com/<owner>/<repo>/<branch>/<path>/SKILL.md
//	<owner>/<repo>[/<path>]
//
// A path that points at a file (has an extension, e.g. SKILL.md) is reduced to
// its directory, since a skill is the whole folder. A missing branch is left
// empty; the contents API then uses the repo's default branch.
func parseSkillRef(s string) (skillRef, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return skillRef{}, errors.New("empty skill reference")
	}

	var owner, repo, ref, p string
	if strings.Contains(s, "github.com") || strings.Contains(s, "githubusercontent.com") {
		if !strings.Contains(s, "://") {
			s = "https://" + s
		}
		u, err := url.Parse(s)
		if err != nil {
			return skillRef{}, fmt.Errorf("bad url: %w", err)
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		switch u.Host {
		case "github.com":
			if len(parts) < 2 {
				return skillRef{}, fmt.Errorf("not a repo url: %s", s)
			}
			owner, repo = parts[0], parts[1]
			if len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "blob") {
				ref, p = parts[3], strings.Join(parts[4:], "/")
			}
		case "raw.githubusercontent.com":
			if len(parts) < 3 {
				return skillRef{}, fmt.Errorf("not a raw url: %s", s)
			}
			owner, repo, ref = parts[0], parts[1], parts[2]
			p = strings.Join(parts[3:], "/")
		default:
			return skillRef{}, fmt.Errorf("unsupported host %q (want github.com or raw.githubusercontent.com)", u.Host)
		}
	} else {
		parts := strings.Split(strings.Trim(s, "/"), "/")
		if len(parts) < 2 {
			return skillRef{}, fmt.Errorf("expected owner/repo[/path], got %q", s)
		}
		owner, repo = parts[0], parts[1]
		p = strings.Join(parts[2:], "/")
	}

	// Reduce a file path to its directory: a skill is the folder, not the file.
	if base := path.Base(p); p != "" && strings.Contains(base, ".") {
		p = strings.Trim(strings.TrimSuffix(p, base), "/")
	}
	return skillRef{owner: owner, repo: repo, ref: ref, dir: p}, nil
}

// Download resolves source to a skill directory and returns every file in it.
func (d *Downloader) Download(ctx context.Context, source string) (*DownloadResult, error) {
	ref, err := parseSkillRef(source)
	if err != nil {
		return nil, err
	}

	var items []ghItem
	if err := d.list(ctx, ref, ref.dir, 0, &items); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no files found for %s/%s at %q", ref.owner, ref.repo, ref.dir)
	}

	// Fetch every file's raw bytes concurrently, bounded by a semaphore. Each
	// goroutine owns its slice index, so no lock is needed to collect results.
	files := make([]SkillFile, len(items))
	sem := make(chan struct{}, maxDownloadConcurrency)
	var wg sync.WaitGroup
	for i, it := range items {
		wg.Add(1)
		go func(i int, it ghItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			sf := SkillFile{Path: it.Path}
			content, err := d.fetchRaw(ctx, it.DownloadURL)
			if err != nil {
				sf.Error = err.Error()
			} else {
				sf.Content = string(content)
				sf.Bytes = len(content)
			}
			files[i] = sf
		}(i, it)
	}
	wg.Wait()

	total := 0
	for _, f := range files {
		total += f.Bytes
	}
	name := path.Base(ref.dir)
	if name == "" || name == "." {
		name = ref.repo
	}
	return &DownloadResult{Name: name, Source: source, Ref: ref.ref, Files: files, TotalBytes: total}, nil
}

type ghItem struct {
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
	Size        int    `json:"size"`
}

// list walks the contents API from dir, appending files to acc and recursing
// into subdirectories up to maxSkillDepth / maxSkillFiles.
func (d *Downloader) list(ctx context.Context, ref skillRef, dir string, depth int, acc *[]ghItem) error {
	if depth > maxSkillDepth || len(*acc) >= maxSkillFiles {
		return nil
	}

	endpoint := d.apiBase + "/repos/" + url.PathEscape(ref.owner) + "/" + url.PathEscape(ref.repo) + "/contents"
	if dir != "" {
		endpoint += "/" + escapePath(dir)
	}
	if ref.ref != "" {
		endpoint += "?ref=" + url.QueryEscape(ref.ref)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build contents request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if d.Token != "" {
		req.Header.Set("Authorization", "Bearer "+d.Token)
	}

	resp, err := d.Client.Do(req)
	if err != nil {
		return fmt.Errorf("github contents: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	if err != nil {
		return fmt.Errorf("read contents: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if msg := decodeGithubError(data); msg != "" {
			return fmt.Errorf("github contents %s/%s: status %d: %s", ref.owner, ref.repo, resp.StatusCode, msg)
		}
		return fmt.Errorf("github contents %s/%s: status %d", ref.owner, ref.repo, resp.StatusCode)
	}

	// The API returns an array for a directory and a single object for a file.
	var items []ghItem
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '[' {
		if err := json.Unmarshal(data, &items); err != nil {
			return fmt.Errorf("decode contents: %w", err)
		}
	} else {
		var one ghItem
		if err := json.Unmarshal(data, &one); err != nil {
			return fmt.Errorf("decode content: %w", err)
		}
		items = []ghItem{one}
	}

	for _, it := range items {
		if len(*acc) >= maxSkillFiles {
			break
		}
		switch it.Type {
		case "file":
			*acc = append(*acc, it)
		case "dir":
			if err := d.list(ctx, ref, it.Path, depth+1, acc); err != nil {
				return err
			}
		}
	}
	return nil
}

// fetchRaw downloads one raw file. It refuses any host outside GitHub: the
// download_url comes from GitHub's API and is always on githubusercontent.com,
// so this both documents the invariant and blocks an unexpected redirect target.
func (d *Downloader) fetchRaw(ctx context.Context, rawURL string) ([]byte, error) {
	if rawURL == "" {
		return nil, errors.New("no download url")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("bad download url: %w", err)
	}
	if h := u.Hostname(); !d.skipHostCheck && !strings.HasSuffix(h, "githubusercontent.com") {
		return nil, fmt.Errorf("refusing non-github host %q", h)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build raw request: %w", err)
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch raw: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch raw: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read raw: %w", err)
	}
	if len(data) > maxFileBytes {
		data = append(data[:maxFileBytes], []byte("\n... [truncated]")...)
	}
	return data, nil
}

// escapePath path-escapes each segment while preserving the slashes.
func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// decodeGithubError best-effort extracts the "message" field of a GitHub error.
func decodeGithubError(data []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &e)
	return e.Message
}
