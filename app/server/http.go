package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
)

type site struct {
	index    contentIndex
	gitSHA   string
	draining atomic.Bool
}

func newSite(index contentIndex, gitSHA string) *site {
	return &site{index: index, gitSHA: gitSHA}
}

func (s *site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	first, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	switch first {
	case "api":
		s.api(w, r)
	case "livez", "readyz":
		s.probe(w, r)
	default:
		writeText(w, r, http.StatusNotFound, "Not found.\n")
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
