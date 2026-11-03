package lib

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestBlobRateLimitPreservesRetryAfter(t *testing.T) {
	var fallback atomic.Bool
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" && fallback.Load() {
			w.WriteHeader(405)
			return
		}
		w.Header().Set("Retry-After", "120")
		http.Error(w, "private rejection", 429)
	}))
	defer upstream.Close()
	config := NewConfig()
	u, _ := url.Parse(upstream.URL)
	config.SetUrl(*u)
	config.SetHttpClient(upstream.Client())
	api, err := NewRegistryApi(config)
	if err != nil {
		t.Fatal(err)
	}
	ref := EmptyRefspec()
	if err := ref.Set("owner/image:latest"); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"HEAD", "GET", "HEAD-fallback"} {
		var err error
		fallback.Store(method == "HEAD-fallback")
		if method != "GET" {
			_, _, _, err = api.BlobInfo(context.Background(), ref, 2, "sha256:object", nil)
		} else {
			_, err = api.RangeBlobs(context.Background(), ref, 2, "sha256:object", 0, -1, nil)
		}
		var quota interface {
			error
			HTTPStatus() int
			RetryAfter() string
		}
		if !errors.As(err, &quota) || quota.HTTPStatus() != 429 || quota.RetryAfter() != "120" {
			t.Fatalf("%s lost rate limit contract: %T %v", method, err, err)
		}
	}
}

func TestManifestRateLimitPreservesRetryAfter(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(429)
	}))
	defer upstream.Close()
	cfg := NewConfig()
	u, _ := url.Parse(upstream.URL)
	cfg.SetUrl(*u)
	cfg.SetHttpClient(upstream.Client())
	api, err := NewRegistryApi(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ref := NewRefspec("owner/image", "latest")
	for _, head := range []bool{false, true} {
		_, err := api.Manifests(context.Background(), head, ref, 2, nil)
		var quota interface {
			error
			HTTPStatus() int
			RetryAfter() string
		}
		if !errors.As(err, &quota) || quota.HTTPStatus() != 429 || quota.RetryAfter() != "10" {
			t.Fatalf("manifest head=%t quota lost: %T %v", head, err, err)
		}
	}
}
