package main

import (
	"encoding/json"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
)

var hashedAsset = regexp.MustCompile(`-[A-Za-z0-9_-]{8,}\.[^.]+$`)

type site struct {
	index    contentIndex
	dist     *os.Root
	gitSHA   string
	draining atomic.Bool
}

func newSite(index contentIndex, dist *os.Root, gitSHA string) *site {
	return &site{index: index, dist: dist, gitSHA: gitSHA}
}

func (s *site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	first, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	switch first {
	case "api":
		s.api(w, r)
	case "media":
		writeText(w, r, http.StatusNotFound, "Media not found.\n")
	case "livez", "readyz":
		s.probe(w, r)
	default:
		s.frontend(w, r)
	}
}

func (s *site) api(w http.ResponseWriter, r *http.Request) {
	var value any
	var missing string
	cache := "no-cache"
	switch {
	case r.URL.Path == "/api/v1/version":
		value = struct {
			GitSHA string `json:"gitSha"`
		}{s.gitSHA}
		cache = "no-store"
	case r.URL.Path == "/api/v1/writing":
		value = s.index["writing"].summaries
	case strings.HasPrefix(r.URL.Path, "/api/v1/writing/"):
		slug := strings.TrimPrefix(r.URL.Path, "/api/v1/writing/")
		if !slugPattern.MatchString(slug) {
			writeAPIError(w, r, 404, "not_found", "Writing item not found.")
			return
		}
		if item, exists := s.index["writing"].items[slug]; exists {
			value = item
		} else {
			missing = "Writing item not found."
		}
	default:
		writeAPIError(w, r, 404, "not_found", "API route not found.")
		return
	}
	w.Header().Set("Cache-Control", cache)
	if !allowRead(w, r) {
		writeAPIError(w, r, 405, "method_not_allowed", "Method not allowed.")
		return
	}
	if missing != "" {
		writeAPIError(w, r, 404, "not_found", missing)
		return
	}
	writeJSON(w, r, http.StatusOK, value)
}

func allowRead(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	return false
}

func writeAPIError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, r, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		log.Printf("encode API response: %v", err)
		status = http.StatusInternalServerError
		body = []byte(`{"error":{"code":"internal_error","message":"Internal server error."}}`)
	}
	writeResponse(w, r, status, "application/json; charset=utf-8", append(body, '\n'))
}

func writeText(w http.ResponseWriter, r *http.Request, status int, body string) {
	writeResponse(w, r, status, "text/plain; charset=utf-8", []byte(body))
}

func writeResponse(w http.ResponseWriter, r *http.Request, status int, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func (s *site) probe(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path != "/livez" && r.URL.Path != "/readyz":
		writeText(w, r, 404, "Probe not found.\n")
	case !allowRead(w, r):
		writeText(w, r, 405, "Method not allowed.\n")
	case r.URL.Path == "/readyz" && s.draining.Load():
		writeText(w, r, 503, "Draining.\n")
	default:
		writeText(w, r, 200, "ok\n")
	}
}

func (s *site) frontend(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if s.dist == nil || strings.ContainsAny(name, "\\:\x00") ||
		(name != "" && (path.Clean(name) != name || name == "." || name == ".." || strings.HasPrefix(name, "../"))) {
		writeText(w, r, 404, "Not found.\n")
		return
	}
	if name != "" && s.serveFile(w, r, name) {
		return
	}
	// Vite's assets directory and file-looking URLs never fall back to the shell.
	if name != "assets" && !strings.HasPrefix(name, "assets/") && path.Ext(name) == "" {
		w.Header().Add("Vary", "Accept")
		if acceptsHTML(r.Header.Values("Accept")) && s.serveFile(w, r, "index.html") {
			return
		}
	}
	writeText(w, r, 404, "Not found.\n")
}

func acceptsHTML(headers []string) bool {
	for _, header := range headers {
		for _, entry := range strings.Split(header, ",") {
			mediaType, params, err := mime.ParseMediaType(entry)
			if err != nil || mediaType != "text/html" {
				continue
			}
			quality, specified := params["q"]
			if !specified {
				return true
			}
			if q, err := strconv.ParseFloat(quality, 64); err == nil && q > 0 && q <= 1 {
				return true
			}
		}
	}
	return false
}

func (s *site) serveFile(w http.ResponseWriter, r *http.Request, name string) bool {
	info, err := s.dist.Stat(name)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	file, err := s.dist.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	if !allowRead(w, r) {
		writeText(w, r, 405, "Method not allowed.\n")
		return true
	}
	cache := "no-cache"
	if strings.HasPrefix(name, "assets/") && hashedAsset.MatchString(path.Base(name)) {
		cache = "public, max-age=31536000, immutable"
	}
	w.Header().Set("Cache-Control", cache)
	http.ServeContent(w, r, path.Base(name), info.ModTime(), file)
	return true
}
