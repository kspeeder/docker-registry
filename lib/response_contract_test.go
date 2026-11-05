package lib

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/kspeeder/docker-registry/lib/connector"
)

type contractBody struct {
	io.Reader
	closes int
}

func (b *contractBody) Close() error { b.closes++; return nil }

type contractConnector struct {
	do func(string, map[string]string) (*http.Response, error)
}

func (c contractConnector) Request(_ context.Context, method string, _ *url.URL, headers map[string]string, _ string) (*http.Response, error) {
	return c.do(method, headers)
}
func (c contractConnector) Get(ctx context.Context, u *url.URL, h map[string]string, hint string) (*http.Response, error) {
	return c.Request(ctx, http.MethodGet, u, h, hint)
}
func (c contractConnector) Head(ctx context.Context, u *url.URL, h map[string]string, hint string) (*http.Response, error) {
	return c.Request(ctx, http.MethodHead, u, h, hint)
}
func (c contractConnector) Delete(ctx context.Context, u *url.URL, h map[string]string, hint string) (*http.Response, error) {
	return c.Request(ctx, http.MethodDelete, u, h, hint)
}
func (c contractConnector) GetStatistics() connector.Statistics { return nil }

func contractAPI(t *testing.T, do func(string, map[string]string) (*http.Response, error)) *registryApi {
	t.Helper()
	u, err := url.Parse("https://registry.example.test")
	if err != nil {
		t.Fatal(err)
	}
	cfg := NewConfig()
	cfg.SetUrl(*u)
	return &registryApi{cfg: cfg, connector: contractConnector{do: do}}
}

func TestRangeBlobsValidatesExactRepresentation(t *testing.T) {
	for _, tc := range []struct {
		name, span, length, requested string
		status                        int
		start, end                    int64
		valid                         bool
	}{
		{"exact", "bytes 4-7/16", "4", "bytes=4-7", 206, 4, 8, true},
		{"ignored-range", "", "16", "bytes=4-7", 200, 4, 8, false},
		{"wrong-offset", "bytes 0-3/16", "4", "bytes=4-7", 206, 4, 8, false},
		{"wrong-end", "bytes 4-8/16", "5", "bytes=4-7", 206, 4, 8, false},
		{"wrong-length", "bytes 4-7/16", "3", "bytes=4-7", 206, 4, 8, false},
		{"impossible-total", "bytes 4-7/7", "4", "bytes=4-7", 206, 4, 8, false},
		{"missing-span", "", "4", "bytes=4-7", 206, 4, 8, false},
		{"open-range", "bytes 4-15/16", "12", "bytes=4-", 206, 4, -1, true},
		{"partial-full-download", "bytes 4-7/16", "4", "", 206, 0, -1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &contractBody{Reader: strings.NewReader("fixture")}
			api := contractAPI(t, func(_ string, h map[string]string) (*http.Response, error) {
				if h["Range"] != tc.requested {
					t.Errorf("Range=%q, want %q", h["Range"], tc.requested)
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Range": {tc.span}, "Content-Length": {tc.length}}, Body: body}, nil
			})
			resp, err := api.RangeBlobs(context.Background(), NewRefspec("owner/image", "latest"), 2, "sha256:test", tc.start, tc.end, nil)
			if (err == nil) != tc.valid {
				t.Errorf("valid=%v, got response=%v err=%v", tc.valid, resp != nil, err)
			}
			if tc.valid && resp != nil {
				if body.closes != 0 {
					t.Error("successful response closed before ownership transfer")
				}
				resp.Body.Close()
			}
			if body.closes != 1 {
				t.Errorf("response body closed %d times", body.closes)
			}
		})
	}
}

func TestRegistryPreservesQuotaMetadata(t *testing.T) {
	for _, name := range []string{"manifest-get", "manifest-head", "blob-head", "blob-head-fallback", "blob-range", "blob-full", "tag-details"} {
		t.Run(name, func(t *testing.T) {
			var bodies []*contractBody
			calls := 0
			api := contractAPI(t, func(method string, _ map[string]string) (*http.Response, error) {
				calls++
				status := 429
				if name == "blob-head-fallback" && method == http.MethodHead {
					status = 405
				}
				body := &contractBody{Reader: strings.NewReader("PRIVATE-QUOTA-BODY")}
				bodies = append(bodies, body)
				return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": {"120"}}, Body: body}, nil
			})
			ref := NewRefspec("owner/image", "latest")
			var err error
			switch name {
			case "manifest-get", "manifest-head":
				_, err = api.Manifests(context.Background(), name == "manifest-head", ref, 2, nil)
			case "blob-head", "blob-head-fallback":
				_, _, _, err = api.BlobInfo(context.Background(), ref, 2, "sha256:test", nil)
			case "blob-range":
				_, err = api.RangeBlobs(context.Background(), ref, 2, "sha256:test", 0, 1, nil)
			case "blob-full":
				_, err = api.GetBlobs(context.Background(), ref, 2, "sha256:test")
			case "tag-details":
				_, err = api.GetTagDetails(context.Background(), ref, 2)
			}
			var quota interface {
				HTTPStatus() int
				RetryAfter() string
			}
			if !errors.As(err, &quota) || quota.HTTPStatus() != 429 || quota.RetryAfter() != "120" {
				t.Errorf("quota metadata lost: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "PRIVATE") {
				t.Error("quota error disclosed upstream body")
			}
			want := 1
			if name == "blob-head-fallback" {
				want = 2
			}
			if calls != want {
				t.Errorf("requests=%d, want %d", calls, want)
			}
			for _, body := range bodies {
				if body.closes != 1 {
					t.Errorf("discarded response closed %d times", body.closes)
				}
			}
		})
	}
}

func TestPaginationAndDeleteClosePersistentResponseBodies(t *testing.T) {
	for _, outcome := range []string{"page-success", "page-decode-error", "page-status-error", "delete"} {
		t.Run(outcome, func(t *testing.T) {
			payload, status := `{"repositories":["owner/image"]}`, 200
			if outcome == "page-decode-error" {
				payload = "{"
			}
			if outcome == "page-status-error" {
				status = 500
			}
			if outcome == "delete" {
				status = 202
			}
			body := &contractBody{Reader: strings.NewReader(payload)}
			api := contractAPI(t, func(string, map[string]string) (*http.Response, error) {
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: body, Close: false}, nil
			})
			if outcome == "delete" {
				if err := api.DeleteTag(NewRefspec("owner/image", "latest")); err != nil {
					t.Fatal(err)
				}
			} else {
				page := api.ListRepositories()
				for range page.Repositories() {
				}
				wantError := outcome != "page-success"
				if (page.LastError() != nil) != wantError {
					t.Fatalf("page error=%v", page.LastError())
				}
			}
			if body.closes != 1 {
				t.Fatalf("persistent response closed %d times", body.closes)
			}
		})
	}
}

type unreadResponseBody struct{ reads, closes int }

func (b *unreadResponseBody) Read([]byte) (int, error) {
	b.reads++
	return 0, errors.New("body must remain unread")
}
func (b *unreadResponseBody) Close() error { b.closes++; return nil }

func TestBlobResponseValidationOnlyInspectsHeaders(t *testing.T) {
	for _, status := range []int{403, 404, 429, 500, 503} {
		body := &unreadResponseBody{}
		resp := &http.Response{StatusCode: status, Header: make(http.Header), Body: body}
		if err := ValidateBlobResponse(resp, 4, 8); err == nil {
			t.Errorf("status %d was accepted", status)
		}
		if body.reads != 0 || body.closes != 0 {
			t.Errorf("status %d: validation read=%d closed=%d", status, body.reads, body.closes)
		}
	}
}
