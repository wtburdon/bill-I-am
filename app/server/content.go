package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type summary struct {
	Slug    string  `json:"slug"`
	Title   string  `json:"title"`
	Date    string  `json:"date"`
	Summary *string `json:"summary,omitempty"`
}

type writing struct {
	summary
	Body string `json:"body"`
}

type collection struct {
	items     map[string]writing
	summaries []summary
}

type contentIndex map[string]collection

func loadContent(root string) (contentIndex, error) {
	const collectionName = "writing"
	const repoRoot = "app/content"
	location := filepath.Join(root, collectionName)
	for _, dir := range []struct{ path, label string }{
		{root, repoRoot}, {location, repoRoot + "/" + collectionName},
	} {
		if err := requirePath(dir.path, true); err != nil {
			return nil, fmt.Errorf("%s: %w", dir.label, err)
		}
	}
	entries, err := os.ReadDir(location)
	if err != nil {
		return nil, fmt.Errorf("%s/%s: %w", repoRoot, collectionName, err)
	}

	writings := newCollection()
	var problems []error
	seen := make(map[string]bool)
	for _, entry := range entries {
		slug := entry.Name()
		itemLabel := repoRoot + "/" + collectionName + "/" + slug
		switch {
		case slug == ".gitkeep" && entry.Type().IsRegular():
			continue
		case entry.Type()&os.ModeSymlink != 0:
			problems = append(problems, fmt.Errorf("%s: symlinks are not allowed", itemLabel))
			continue
		case !entry.IsDir():
			continue
		}

		fileLabel := itemLabel + "/index.md"
		if !slugPattern.MatchString(slug) {
			problems = append(problems, fmt.Errorf("%s: invalid slug %q", fileLabel, slug))
			continue
		}
		if seen[slug] {
			problems = append(problems, fmt.Errorf("%s: duplicate slug %q", fileLabel, slug))
			continue
		}
		seen[slug] = true
		item, err := readWriting(filepath.Join(location, slug, "index.md"), slug)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", fileLabel, err))
			continue
		}
		writings.add(item)
	}
	if err := errors.Join(problems...); err != nil {
		return nil, err
	}
	writings.sort()
	return contentIndex{collectionName: writings}, nil
}

func newCollection() collection {
	return collection{items: make(map[string]writing), summaries: []summary{}}
}

func (c *collection) add(item writing) {
	c.items[item.Slug] = item
	c.summaries = append(c.summaries, item.summary)
}

func (c *collection) sort() {
	slices.SortFunc(c.summaries, func(a, b summary) int {
		if a.Date != b.Date {
			return strings.Compare(b.Date, a.Date)
		}
		return strings.Compare(a.Slug, b.Slug)
	})
}

func requirePath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("symlinks are not allowed")
	}
	if directory && !info.IsDir() {
		return errors.New("expected a directory")
	}
	if !directory && !info.Mode().IsRegular() {
		return errors.New("expected a regular file")
	}
	return nil
}

func readWriting(path, slug string) (writing, error) {
	if err := requirePath(path, false); err != nil {
		return writing{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return writing{}, err
	}
	if !utf8.Valid(data) {
		return writing{}, errors.New("index.md must be UTF-8")
	}
	text := strings.TrimPrefix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\uFEFF")
	opening, rest, _ := strings.Cut(text, "\n")
	if opening != "---" {
		return writing{}, errors.New("missing opening front matter delimiter")
	}
	offset := 0
	for offset < len(rest) {
		line, _, hasNewline := strings.Cut(rest[offset:], "\n")
		if line == "---" {
			body := rest[offset+len(line):]
			body = strings.TrimPrefix(body, "\n")
			return parseWriting(slug, rest[:offset], body)
		}
		if !hasNewline {
			break
		}
		offset += len(line) + 1
	}
	return writing{}, errors.New("missing closing front matter delimiter")
}

func parseWriting(slug, frontMatter, body string) (writing, error) {
	var fields map[string]yaml.Node
	if err := yaml.Unmarshal([]byte(frontMatter), &fields); err != nil {
		return writing{}, fmt.Errorf("invalid YAML front matter: %w", err)
	}
	title := fields["title"]
	if title.Kind != yaml.ScalarNode || title.Tag != "!!str" {
		return writing{}, errors.New("title must be a nonblank string")
	}
	date := fields["date"]
	if date.Kind != yaml.ScalarNode || (date.Tag != "!!str" && date.Tag != "!!timestamp") {
		return writing{}, errors.New("date must be a valid calendar date in YYYY-MM-DD form")
	}
	item := writing{summary: summary{Slug: slug, Title: title.Value, Date: date.Value}, Body: body}
	if err := validateWriting(item); err != nil {
		return writing{}, err
	}
	if value, exists := fields["summary"]; exists {
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
			return writing{}, errors.New("summary must be a string")
		}
		item.Summary = &value.Value
	}
	return item, nil
}

func validateWriting(item writing) error {
	if strings.TrimSpace(item.Title) == "" {
		return errors.New("title must be a nonblank string")
	}
	const layout = "2006-01-02"
	parsed, err := time.Parse(layout, item.Date)
	if err != nil || parsed.Year() == 0 || parsed.Format(layout) != item.Date {
		return errors.New("date must be a valid calendar date in YYYY-MM-DD form")
	}
	if strings.TrimSpace(item.Body) == "" {
		return errors.New("body must not be blank")
	}
	return nil
}
