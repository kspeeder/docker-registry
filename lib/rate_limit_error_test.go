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
