package lib

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/kspeeder/docker-registry/lib/connector"
)

type blobInfoFallbackConnector struct {
	headStatus   int
	getStatus    int
	getHeader    http.Header
	getResponses []blobInfoFallbackResponse
	getRange     string
	getURLs      []string
	getHints     []string
	headCalls    int
	getCalls     int
}

type blobInfoFallbackResponse struct {
	status int
	header http.Header
}

func (c *blobInfoFallbackConnector) Request(ctx context.Context, method string, u *url.URL, headers map[string]string, hint string) (*http.Response, error) {
	_ = ctx
	_ = method
	_ = u
	_ = headers
	_ = hint
	return nil, http.ErrNotSupported
}

func (c *blobInfoFallbackConnector) Delete(ctx context.Context, u *url.URL, headers map[string]string, hint string) (*http.Response, error) {
	_ = ctx
	_ = u
	_ = headers
	_ = hint
	return nil, http.ErrNotSupported
}

func (c *blobInfoFallbackConnector) Get(ctx context.Context, u *url.URL, headers map[string]string, hint string) (*http.Response, error) {
	_ = ctx
	idx := c.getCalls
	c.getCalls++
	c.getURLs = append(c.getURLs, u.String())
	c.getHints = append(c.getHints, hint)
	c.getRange = headers["Range"]
	status := c.getStatus
	header := c.getHeader
	if idx < len(c.getResponses) {
		status = c.getResponses[idx].status
		header = c.getResponses[idx].header
	}
	return &http.Response{
		StatusCode: status,
		Header:     header.Clone(),
		Body:       io.NopCloser(http.NoBody),
	}, nil
}

func (c *blobInfoFallbackConnector) Head(ctx context.Context, u *url.URL, headers map[string]string, hint string) (*http.Response, error) {
	_ = ctx
	_ = u
	_ = headers
	_ = hint
	c.headCalls++
	return &http.Response{
		StatusCode: c.headStatus,
		Header:     make(http.Header),
		Body:       io.NopCloser(http.NoBody),
	}, nil
}

func (c *blobInfoFallbackConnector) GetStatistics() connector.Statistics {
	return nil
}

func TestBlobInfoFallsBackToRangeGetWhenHeadReturnsNotFound(t *testing.T) {
	registryURL, err := url.Parse("https://registry.example.test")
	if err != nil {
		t.Fatalf("parse registry URL: %v", err)
	}
	lastModified := "Wed, 17 Jun 2026 08:00:00 GMT"
	conn := &blobInfoFallbackConnector{
		headStatus: http.StatusNotFound,
		getStatus:  http.StatusPartialContent,
		getHeader: http.Header{
			"Content-Range":  []string{"bytes 0-0/12935042"},
			"Content-Length": []string{"1"},
			"Last-Modified":  []string{lastModified},
		},
	}
	api := &registryApi{
		cfg: Config{
			registryUrl: *registryURL,
		},
		connector: conn,
	}
	ref := NewRefspec("kspeeder/kspeeder", "0.4.1")

	size, modTime, header, err := api.BlobInfo(context.Background(), ref, 2, "sha256:test", nil)
	if err != nil {
		t.Fatalf("BlobInfo returned error: %v", err)
	}
	if size != 12935042 {
		t.Fatalf("expected size 12935042, got %d", size)
	}
	wantModTime, err := time.Parse(time.RFC1123, lastModified)
	if err != nil {
		t.Fatalf("parse expected Last-Modified: %v", err)
	}
	if !modTime.Equal(wantModTime) {
		t.Fatalf("expected modTime %s, got %s", wantModTime, modTime)
	}
	if header.Get("Content-Range") != "bytes 0-0/12935042" {
		t.Fatalf("expected fallback response headers, got %#v", header)
	}
	if conn.headCalls != 1 || conn.getCalls != 1 {
		t.Fatalf("expected one HEAD and one GET, got head=%d get=%d", conn.headCalls, conn.getCalls)
	}
	if conn.getRange != "bytes=0-0" {
		t.Fatalf("expected fallback GET range bytes=0-0, got %q", conn.getRange)
	}
	if len(conn.getHints) != 1 || conn.getHints[0] != "pull:kspeeder/kspeeder" {
		t.Fatalf("expected fallback GET to use blob cache hint, got %#v", conn.getHints)
	}
}

func TestBlobInfoFallbackFollowsRedirectLocation(t *testing.T) {
	registryURL, err := url.Parse("https://registry.example.test")
	if err != nil {
		t.Fatalf("parse registry URL: %v", err)
	}
	conn := &blobInfoFallbackConnector{
		headStatus: http.StatusNotFound,
		getResponses: []blobInfoFallbackResponse{
			{
				status: http.StatusTemporaryRedirect,
				header: http.Header{
					"Location": []string{"https://blob-storage.example.test/blob"},
				},
			},
			{
				status: http.StatusPartialContent,
				header: http.Header{
					"Content-Range": []string{"bytes 0-0/12935042"},
				},
			},
		},
	}
	api := &registryApi{
		cfg: Config{
			registryUrl: *registryURL,
		},
		connector: conn,
	}
	ref := NewRefspec("kspeeder/kspeeder", "0.4.1")

	size, _, _, err := api.BlobInfo(context.Background(), ref, 2, "sha256:test", nil)
	if err != nil {
		t.Fatalf("BlobInfo returned error: %v", err)
	}
	if size != 12935042 {
		t.Fatalf("expected size 12935042, got %d", size)
	}
	if conn.getCalls != 2 {
		t.Fatalf("expected two fallback GET calls, got %d", conn.getCalls)
	}
	if len(conn.getURLs) != 2 || conn.getURLs[1] != "https://blob-storage.example.test/blob" {
		t.Fatalf("expected second GET to follow redirect, got %#v", conn.getURLs)
	}
	if conn.getRange != "bytes=0-0" {
		t.Fatalf("expected redirected GET range bytes=0-0, got %q", conn.getRange)
	}
	if len(conn.getHints) != 2 || conn.getHints[0] != "pull:kspeeder/kspeeder" || conn.getHints[1] != "" {
		t.Fatalf("expected redirect to clear cache hint after first GET, got %#v", conn.getHints)
	}
}

func TestRangeBlobsFollowsRedirectLocation(t *testing.T) {
	registryURL, err := url.Parse("https://registry.example.test")
	if err != nil {
		t.Fatalf("parse registry URL: %v", err)
	}
	conn := &blobInfoFallbackConnector{
		getResponses: []blobInfoFallbackResponse{
			{
				status: http.StatusTemporaryRedirect,
				header: http.Header{
					"Location": []string{"https://blob-storage.example.test/blob"},
				},
			},
			{
				status: http.StatusPartialContent,
				header: http.Header{
					"Content-Range": []string{"bytes 128-255/12935042"},
				},
			},
		},
	}
	api := &registryApi{
		cfg: Config{
			registryUrl: *registryURL,
		},
		connector: conn,
	}
	ref := NewRefspec("kspeeder/kspeeder", "0.4.1")

	resp, err := api.RangeBlobs(context.Background(), ref, 2, "sha256:test", 128, 256, nil)
	if err != nil {
		t.Fatalf("RangeBlobs returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected final 206 response, got %d", resp.StatusCode)
	}
	if conn.getCalls != 2 {
		t.Fatalf("expected two GET calls, got %d", conn.getCalls)
	}
	if len(conn.getURLs) != 2 || conn.getURLs[1] != "https://blob-storage.example.test/blob" {
		t.Fatalf("expected second GET to follow redirect, got %#v", conn.getURLs)
	}
	if conn.getRange != "bytes=128-255" {
		t.Fatalf("expected redirected GET range bytes=128-255, got %q", conn.getRange)
	}
	if len(conn.getHints) != 2 || conn.getHints[0] != "pull:kspeeder/kspeeder" || conn.getHints[1] != "" {
		t.Fatalf("expected redirect to clear cache hint after first GET, got %#v", conn.getHints)
	}
}
