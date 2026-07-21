package skills

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSkillRef(t *testing.T) {
	cases := []struct {
		in                  string
		owner, repo, ref, d string
	}{
		{"anthropics/skills/skills/pdf", "anthropics", "skills", "", "skills/pdf"},
		{"https://github.com/anthropics/skills/tree/main/skills/pdf", "anthropics", "skills", "main", "skills/pdf"},
		{"https://github.com/anthropics/skills/blob/main/skills/pdf/SKILL.md", "anthropics", "skills", "main", "skills/pdf"},
		{"https://raw.githubusercontent.com/anthropics/skills/main/skills/pdf/SKILL.md", "anthropics", "skills", "main", "skills/pdf"},
		{"owner/repo", "owner", "repo", "", ""},
	}
	for _, c := range cases {
		got, err := parseSkillRef(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if got.owner != c.owner || got.repo != c.repo || got.ref != c.ref || got.dir != c.d {
			t.Errorf("%s -> %+v, want {%s %s %s %s}", c.in, got, c.owner, c.repo, c.ref, c.d)
		}
	}

	if _, err := parseSkillRef("not-a-repo"); err == nil {
		t.Error("want error for bare name")
	}
}

// TestDownloadRecursiveConcurrent stubs the GitHub contents API with a nested
// skill (SKILL.md + scripts/build.py) and pins that the downloader recurses,
// fetches every file, and reports the right totals.
func TestDownloadRecursiveConcurrent(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/o/r/contents/skills/demo":
			// top-level dir: one file, one subdir
			writeJSON(w, []ghItem{
				{Path: "skills/demo/SKILL.md", Type: "file", DownloadURL: srv.URL + "/raw/SKILL.md", Size: 10},
				{Path: "skills/demo/scripts", Type: "dir"},
			})
		case r.URL.Path == "/repos/o/r/contents/skills/demo/scripts":
			writeJSON(w, []ghItem{
				{Path: "skills/demo/scripts/build.py", Type: "file", DownloadURL: srv.URL + "/raw/build.py", Size: 5},
			})
		case strings.HasPrefix(r.URL.Path, "/raw/"):
			// download_url points back here; must pass the githubusercontent guard,
			// so rewrite host check by serving from a *.githubusercontent.com alias
			// is impossible in-test -> the downloader is pointed at a test host, so
			// this branch is only reached when the guard is relaxed for tests below.
			_, _ = w.Write([]byte("content-of " + r.URL.Path))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	d := NewDownloader()
	d.apiBase = srv.URL
	d.skipHostCheck = true // test stub isn't on githubusercontent.com

	res, err := d.Download(context.Background(), "o/r/skills/demo")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if len(res.Files) != 2 {
		t.Fatalf("got %d files, want 2: %+v", len(res.Files), res.Files)
	}
	if res.Name != "demo" {
		t.Errorf("name = %q, want demo", res.Name)
	}
	got := map[string]string{}
	for _, f := range res.Files {
		if f.Error != "" {
			t.Errorf("file %s errored: %s", f.Path, f.Error)
		}
		got[f.Path] = f.Content
	}
	if !strings.Contains(got["skills/demo/SKILL.md"], "SKILL.md") ||
		!strings.Contains(got["skills/demo/scripts/build.py"], "build.py") {
		t.Errorf("unexpected contents: %+v", got)
	}
}

// TestDownloadRejectsNonGithubHost pins the SSRF guard on raw fetches.
func TestDownloadRejectsNonGithubHost(t *testing.T) {
	d := NewDownloader()
	if _, err := d.fetchRaw(context.Background(), "https://evil.example.com/x"); err == nil {
		t.Fatal("want refusal for non-github host, got nil")
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
