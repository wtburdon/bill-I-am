package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/transport/http"
)

const maxReleaseJSON = 64 << 20

type releaseBucket struct {
	client *s3.Client
	name   string
}

func bucketFromEnv() (releaseBucket, error) {
	bucket, endpoint := os.Getenv("R2_BUCKET"), os.Getenv("R2_ENDPOINT")
	id, secret := os.Getenv("R2_ACCESS_KEY_ID"), os.Getenv("R2_SECRET_ACCESS_KEY")
	if bucket == "" || endpoint == "" || id == "" || secret == "" {
		return releaseBucket{}, errors.New("R2_BUCKET, R2_ENDPOINT, R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY are required")
	}
	u, err := url.Parse(endpoint)
	if err != nil || !validR2Endpoint(u) {
		return releaseBucket{}, errors.New("R2_ENDPOINT must be an HTTPS origin or loopback HTTP origin")
	}
	client := s3.New(s3.Options{
		Region:                     "auto",
		BaseEndpoint:               aws.String(endpoint),
		UsePathStyle:               true,
		RetryMaxAttempts:           3,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: id, SecretAccessKey: secret}, nil
		}),
	})
	return releaseBucket{client: client, name: bucket}, nil
}

func validR2Endpoint(u *url.URL) bool {
	if u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if u.Scheme == "https" {
		return u.Path == "" || u.Path == "/"
	}
	loopback := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	return u.Scheme == "http" && loopback &&
		(u.Path == "" || u.Path == "/" || u.Path == "/cdn-cgi/local/r2/s3")
}

func (b releaseBucket) get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := b.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &b.name, Key: &key})
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", key, err)
	}
	return obj.Body, nil
}

func loadR2Snapshot(b releaseBucket, sha string) (contentIndex, error) {
	if !releaseSHA.MatchString(sha) {
		return nil, errors.New("R2_RELEASE_SHA must be a full lowercase Git SHA")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, err := b.get(ctx, releaseKey(sha, "snapshot.json"))
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, maxReleaseJSON+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxReleaseJSON {
		return nil, errors.New("snapshot exceeds the size limit")
	}
	return loadSnapshot(data, sha)
}

func publishRelease(ctx context.Context, b releaseBucket, dir, sha string) error {
	if !releaseSHA.MatchString(sha) {
		return errors.New("release SHA must be a full lowercase Git SHA")
	}
	read := func(key string) ([]byte, error) {
		return os.ReadFile(filepath.Join(dir, filepath.FromSlash(key)))
	}
	manifestKey, snapshotKey := releaseKey(sha, "manifest.json"), releaseKey(sha, "snapshot.json")
	manifestBytes, err := read(manifestKey)
	if err != nil {
		return err
	}
	var manifest mediaManifest
	if err := decodeReleaseJSON(manifestBytes, &manifest); err != nil {
		return err
	}
	if manifest.Version != releaseFormatVersion || manifest.GitSHA != sha || manifest.Media == nil {
		return errors.New("invalid release manifest")
	}
	snapshotBytes, err := read(snapshotKey)
	if err != nil {
		return err
	}
	if len(snapshotBytes) > maxReleaseJSON {
		return errors.New("snapshot exceeds the size limit")
	}
	if _, err := loadSnapshot(snapshotBytes, sha); err != nil {
		return fmt.Errorf("invalid snapshot: %w", err)
	}

	blobs := make(map[string]string)
	for path, media := range manifest.Media {
		hash, err := hex.DecodeString(media.SHA256)
		if err != nil || len(hash) != sha256.Size || media.SHA256 != strings.ToLower(media.SHA256) ||
			media.Key != "blobs/"+media.SHA256 || !strings.HasPrefix(path, "/media/") ||
			media.ContentType == "" || media.ContentType != mediaTypes[strings.ToLower(filepath.Ext(path))] {
			return fmt.Errorf("invalid media manifest entry %q", path)
		}
		blobs[media.Key] = media.SHA256
	}
	keys := make([]string, 0, len(blobs))
	for key := range blobs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		data, err := read(key)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != blobs[key] {
			return fmt.Errorf("exported blob does not match hash: %s", key)
		}
		if err := b.putAndVerify(ctx, key, "application/octet-stream", data); err != nil {
			return err
		}
	}
	if err := b.putAndVerify(ctx, manifestKey, "application/json", manifestBytes); err != nil {
		return err
	}
	return b.putAndVerify(ctx, snapshotKey, "application/json", snapshotBytes)
}

func (b releaseBucket) putAndVerify(ctx context.Context, key, contentType string, data []byte) error {
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &b.name, Key: &key, Body: bytes.NewReader(data),
		IfNoneMatch: aws.String("*"), ContentType: &contentType,
	})
	if err != nil {
		var response *http.ResponseError
		if !errors.As(err, &response) || response.HTTPStatusCode() != 412 {
			return fmt.Errorf("put %s: %w", key, err)
		}
	}
	body, err := b.get(ctx, key)
	if err != nil {
		return err
	}
	defer body.Close()
	stored := sha256.New()
	n, err := io.Copy(stored, io.LimitReader(body, int64(len(data))+1))
	if err != nil {
		return fmt.Errorf("verify %s: %w", key, err)
	}
	want := sha256.Sum256(data)
	if n != int64(len(data)) || !bytes.Equal(stored.Sum(nil), want[:]) {
		return fmt.Errorf("conflicting bytes at %s", key)
	}
	return nil
}
