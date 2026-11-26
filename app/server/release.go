package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const releaseFormatVersion = 1

var releaseSHA = regexp.MustCompile(`^[a-f0-9]{40}$`)
var mediaFilename = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type writingSnapshot struct {
	Version int       `json:"version"`
	GitSHA  string    `json:"gitSha"`
	Writing []writing `json:"writing"`
}

type mediaManifest struct {
	Version int                    `json:"version"`
	GitSHA  string                 `json:"gitSha"`
	Media   map[string]mediaObject `json:"media"`
}

type mediaObject struct {
	Key         string `json:"key"`
	ContentType string `json:"contentType"`
	SHA256      string `json:"sha256"`
}

var mediaTypes = map[string]string{
	".avif": "image/avif", ".gif": "image/gif", ".jpeg": "image/jpeg", ".jpg": "image/jpeg",
	".png": "image/png", ".webp": "image/webp", ".mp3": "audio/mpeg", ".ogg": "audio/ogg",
	".wav": "audio/wav", ".mp4": "video/mp4", ".webm": "video/webm",
}

func releaseKey(sha, name string) string {
	return "releases/" + sha + "/" + name
}

func exportRelease(index contentIndex, root, out, sha string) error {
	if !releaseSHA.MatchString(sha) {
		return errors.New("release SHA must be a full lowercase Git SHA")
	}
	writings := index["writing"]
	snapshot := writingSnapshot{releaseFormatVersion, sha, make([]writing, 0, len(writings.items))}
	for _, item := range writings.summaries {
		snapshot.Writing = append(snapshot.Writing, writings.items[item.Slug])
	}
	manifest := mediaManifest{releaseFormatVersion, sha, make(map[string]mediaObject)}
	if err := collectMedia(root, out, manifest.Media); err != nil {
		return err
	}
	if err := writeReleaseJSON(out, releaseKey(sha, "manifest.json"), manifest); err != nil {
		return err
	}
	return writeReleaseJSON(out, releaseKey(sha, "snapshot.json"), snapshot)
}

func collectMedia(root, out string, media map[string]mediaObject) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s: symlinks are not allowed", path)
		}
		if entry.IsDir() {
			return nil
		}
		contentType := mediaTypes[strings.ToLower(filepath.Ext(path))]
		if contentType == "" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) != 3 || !slugPattern.MatchString(parts[0]) ||
			!slugPattern.MatchString(parts[1]) || !mediaFilename.MatchString(parts[2]) {
			return fmt.Errorf("%s: expected a media path collection/slug/filename", path)
		}
		if err := requirePath(path, false); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		key := "blobs/" + hash
		if err := writeReleaseFile(out, key, data); err != nil {
			return err
		}
		media["/media/"+strings.Join(parts, "/")] = mediaObject{key, contentType, hash}
		return nil
	})
}

func writeReleaseJSON(out, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeReleaseFile(out, key, append(data, '\n'))
}

func writeReleaseFile(out, key string, data []byte) error {
	path := filepath.Join(out, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func decodeReleaseJSON(data []byte, value any) error {
	if !utf8.Valid(data) {
		return errors.New("release JSON must be UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("unexpected data after release JSON")
	}
	return nil
}

func loadSnapshot(data []byte, expectedSHA string) (contentIndex, error) {
	var snapshot writingSnapshot
	if err := decodeReleaseJSON(data, &snapshot); err != nil {
		return nil, err
	}
	if !releaseSHA.MatchString(expectedSHA) || snapshot.GitSHA != expectedSHA {
		return nil, fmt.Errorf("snapshot SHA %q does not match release %q", snapshot.GitSHA, expectedSHA)
	}
	if snapshot.Version != releaseFormatVersion || snapshot.Writing == nil {
		return nil, errors.New("unsupported snapshot version or missing writing array")
	}
	items := newCollection()
	for _, item := range snapshot.Writing {
		if !slugPattern.MatchString(item.Slug) {
			return nil, fmt.Errorf("invalid snapshot writing slug %q", item.Slug)
		}
		if _, exists := items.items[item.Slug]; exists {
			return nil, fmt.Errorf("duplicate snapshot writing slug %q", item.Slug)
		}
		if err := validateWriting(item); err != nil {
			return nil, fmt.Errorf("snapshot writing %q: %w", item.Slug, err)
		}
		items.add(item)
	}
	items.sort()
	return contentIndex{"writing": items}, nil
}
