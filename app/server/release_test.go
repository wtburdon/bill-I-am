package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

const releaseA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const releaseB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestR2SnapshotRetriesTransientS3Response(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/fixture/"+releaseKey(releaseA, "snapshot.json") {
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
			http.NotFound(w, req)
			return
		}
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "<Error><Code>ServiceUnavailable</Code></Error>")
			return
		}
		_, _ = io.WriteString(w, `{"version":1,"gitSha":"`+releaseA+`","writing":[]}`)
	}))
	defer server.Close()
	t.Setenv("R2_ENDPOINT", server.URL)
	t.Setenv("R2_BUCKET", "fixture")
	t.Setenv("R2_ACCESS_KEY_ID", "test-access")
	t.Setenv("R2_SECRET_ACCESS_KEY", "test-secret")
	bucket, err := bucketFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadR2Snapshot(bucket, releaseA); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected a single retry, got %d attempts", attempts.Load())
	}
}

func TestExportRoundTrip(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	writeFixture(t, root, "writing/later/index.md", poem)
	writeFixture(t, root, "writing/earlier/index.md", poem)
	writeFixture(t, root, "writing/later/photo.png", "fixture image")
	index, err := loadContent(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := exportRelease(index, root, out, releaseA); err != nil {
		t.Fatal(err)
	}
	read := func(key string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(key)))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	snapshot := read("releases/" + releaseA + "/snapshot.json")
	loaded, err := loadSnapshot(snapshot, releaseA)
	if err != nil {
		t.Fatal(err)
	}
	collection := loaded["writing"]
	if len(collection.summaries) != 2 || collection.summaries[0].Slug != "earlier" ||
		collection.items["later"].Body != poemBody {
		t.Fatalf("unexpected writing collection: %+v", collection)
	}
	if _, err := loadSnapshot(snapshot, releaseB); err == nil {
		t.Fatal("accepted a snapshot for the wrong SHA")
	}
	var manifest mediaManifest
	if err := json.Unmarshal(read("releases/"+releaseA+"/manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	media := manifest.Media["/media/writing/later/photo.png"]
	if manifest.GitSHA != releaseA || len(manifest.Media) != 1 || media.ContentType != "image/png" ||
		media.Key != "blobs/"+media.SHA256 || string(read(media.Key)) != "fixture image" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
}

func TestEmptyAndInvalidSnapshots(t *testing.T) {
	empty := []byte(`{"version":1,"gitSha":"` + releaseA + `","writing":[]}`)
	index, err := loadSnapshot(empty, releaseA)
	if err != nil {
		t.Fatal(err)
	}
	if items := index["writing"].summaries; items == nil || len(items) != 0 {
		t.Fatalf("empty writing was not preserved: %+v", items)
	}
	for _, snapshot := range []string{
		`{"version":2,"gitSha":"` + releaseA + `","writing":[]}`,
		`{"version":1,"gitSha":"` + releaseA + `","writing":null}`,
		`{"version":1,"gitSha":"` + releaseA + `","writing":[{"slug":"a","title":"A","date":"2026-02-30","body":"Text"}]}`,
		string(empty) + "{}",
	} {
		if _, err := loadSnapshot([]byte(snapshot), releaseA); err == nil {
			t.Errorf("accepted invalid snapshot: %s", snapshot)
		}
	}
}
