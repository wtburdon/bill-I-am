package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

const poemBody = "First light\nacross the sill.\n\nA bird sings.\n"
const poem = "---\ntitle: First Light\ndate: \"2026-11-25\"\nsummary: A morning poem.\n---\n" + poemBody

func writeFixture(t *testing.T, root, path, content string) {
	t.Helper()
	target := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidContentFailsValidationAndStartup(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "writing/first-light/index.md", strings.Replace(poem, "2026-11-25", "2026-02-30", 1))
	writeFixture(t, root, "writing/no-title/index.md", strings.Replace(poem, "First Light", "", 1))
	for _, command := range []string{"validate", "serve"} {
		err := run([]string{command, "-content-dir", root}, &bytes.Buffer{})
		if err == nil {
			t.Fatalf("%s: accepted invalid content", command)
		}
		for _, want := range []string{"app/content/writing/first-light/index.md", "YYYY-MM-DD", "app/content/writing/no-title/index.md", "title"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error %q is missing %q", command, err, want)
			}
		}
	}
}

func TestWritingAPI(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "writing/zebra/index.md", strings.ReplaceAll(poem, "\n", "\r\n"))
	writeFixture(t, root, "writing/first-light/index.md", strings.ReplaceAll(poem, "summary: A morning poem.\n", ""))
	writeFixture(t, root, "writing/older/index.md", strings.ReplaceAll(poem, "2026-11-25", "2000-01-01"))
	if err := run([]string{"validate", "-content-dir", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	index, err := loadContent(root)
	if err != nil {
		t.Fatal(err)
	}
	server := newSite(index, nil, "dev")
	res := request(server, "GET", "/api/v1/writing", "")
	var list []map[string]string
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &list) != nil || len(list) != 3 {
		t.Fatalf("list: %d %s", res.Code, res.Body)
	}
	if list[0]["slug"] != "first-light" || list[1]["slug"] != "zebra" || list[2]["slug"] != "older" {
		t.Errorf("unexpected order: %v", list)
	}
	if _, exists := list[0]["summary"]; exists {
		t.Errorf("unexpected summary: %v", list[0])
	}
	if _, exists := list[1]["body"]; exists {
		t.Errorf("unexpected body: %v", list[1])
	}
	detail := request(server, "GET", "/api/v1/writing/zebra", "")
	var item map[string]string
	if detail.Code != 200 || json.Unmarshal(detail.Body.Bytes(), &item) != nil ||
		item["body"] != poemBody || item["summary"] != "A morning poem." ||
		item["date"] != "2026-11-25" || item["title"] != "First Light" {
		t.Fatalf("detail: %d %s", detail.Code, detail.Body)
	}
	head := request(server, "HEAD", "/api/v1/writing/zebra", "")
	if head.Code != detail.Code || head.Body.Len() != 0 || head.Header().Get("Cache-Control") != "no-cache" ||
		head.Header().Get("Content-Type") != detail.Header().Get("Content-Type") ||
		head.Header().Get("Content-Length") != detail.Header().Get("Content-Length") {
		t.Errorf("HEAD: %d %v %q", head.Code, head.Header(), head.Body.String())
	}
	for _, tc := range []struct {
		method, path, code string
		status             int
	}{
		{"GET", "/api/v1/writing/missing", "not_found", 404},
		{"GET", "/api/v1/Writing", "not_found", 404},
		{"POST", "/api/v1/writing/zebra", "method_not_allowed", 405},
	} {
		res := request(server, tc.method, tc.path, "text/html")
		if res.Code != tc.status || !strings.Contains(res.Body.String(), `"code":"`+tc.code+`"`) {
			t.Errorf("%s %s: %d %s", tc.method, tc.path, res.Code, res.Body)
		}
		if tc.status == 405 && res.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("Allow: %v", res.Header())
		}
	}
}

func TestEmptyCollection(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "writing/.gitkeep", "")
	index, err := loadContent(root)
	if err != nil {
		t.Fatal(err)
	}
	res := request(newSite(index, nil, "dev"), "GET", "/api/v1/writing", "")
	if res.Code != 200 || strings.TrimSpace(res.Body.String()) != "[]" {
		t.Fatalf("empty list: %d %s", res.Code, res.Body)
	}
}

func TestRejectsSymlinkedPublishedFile(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	writeFixture(t, external, "poem.md", poem)
	writeFixture(t, root, "writing/first-light/.gitkeep", "")
	target := filepath.Join(root, "writing", "first-light", "index.md")
	if err := os.Symlink(filepath.Join(external, "poem.md"), target); err != nil {
		const privilegeNotHeld = syscall.Errno(1314)
		if os.IsPermission(err) || (runtime.GOOS == "windows" && errors.Is(err, privilegeNotHeld)) {
			t.Skipf("creating symlinks requires additional privileges: %v", err)
		}
		t.Fatal(err)
	}
	_, err := loadContent(root)
	if err == nil || !strings.Contains(err.Error(), "app/content/writing/first-light/index.md: symlinks are not allowed") {
		t.Fatalf("expected a path-specific symlink error, got %v", err)
	}
}

func TestStaticRoutesAndProbes(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "index.html", "<!doctype html><title>Site</title>")
	writeFixture(t, root, "assets/main-D3Lkc82f.js", "console.log('site')")
	dist, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer dist.Close()

	const sha = "0123456789abcdef0123456789abcdef01234567"
	site := newSite(nil, dist, sha)
	for _, tc := range []struct {
		path        string
		status      int
		cache, body string
	}{
		{"/writing/first-light", 200, "no-cache", "<title>Site</title>"},
		{"/assets/main-D3Lkc82f.js", 200, "public, max-age=31536000, immutable", "console.log"},
		{"/assets/missing", 404, "", ""},
		{"/missing.js", 404, "", ""},
		{"/api/unknown", 404, "", "not_found"},
		{"/media/writing/first-light/photo.jpg", 404, "", ""},
		{"/livez", 200, "", "ok"},
		{"/readyz", 200, "", "ok"},
		{"/api/v1/version", 200, "no-store", sha},
	} {
		res := request(site, "GET", tc.path, "text/html")
		if res.Code != tc.status || !strings.Contains(res.Body.String(), tc.body) ||
			(tc.cache != "" && res.Header().Get("Cache-Control") != tc.cache) {
			t.Errorf("%s: %d %v %s", tc.path, res.Code, res.Header(), res.Body)
		}
	}
	site.draining.Store(true)
	if got := request(site, "HEAD", "/readyz", ""); got.Code != 503 || got.Body.Len() != 0 {
		t.Errorf("draining readiness: %d %q", got.Code, got.Body)
	}
}

func request(handler http.Handler, method, path, accept string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Accept", accept)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}
