package lib

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type rangeContractRT func(*http.Request) (*http.Response, error)

func (f rangeContractRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type rangeContractBody struct {
	io.Reader
	closes int
}

func (b *rangeContractBody) Close() error { b.closes++; return nil }

func TestBlobResponseContract(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		span       string
		length     int64
		body       string
		start, end int64
		wantErr    bool
		short      bool
	}{
		{"valid", 206, "bytes 4-7/16", 4, "4567", 4, 8, false, false},
		{"unknown-total", 206, "bytes 4-7/*", 4, "4567", 4, 8, false, false},
		{"clipped-at-eof", 206, "bytes 4-7/8", 4, "4567", 4, 12, false, false},
		{"chunked", 206, "bytes 4-7/16", -1, "4567", 4, 8, false, false},
		{"ignored-range", 200, "", 16, "0123456789abcdef", 4, 8, true, false},
		{"wrong-offset", 206, "bytes 0-3/16", 4, "0123", 4, 8, true, false},
		{"wrong-end", 206, "bytes 4-6/16", 3, "456", 4, 8, true, false},
		{"missing-range", 206, "", 4, "4567", 4, 8, true, false},
		{"wrong-total", 206, "bytes 4-7/7", 4, "4567", 4, 8, true, false},
		{"trailing-junk", 206, "bytes 4-7/16 junk", 4, "4567", 4, 8, true, false},
		{"wrong-length", 206, "bytes 4-7/16", 3, "456", 4, 8, true, false},
		{"short-body", 206, "bytes 4-7/16", 4, "45", 4, 8, false, true},
		{"zero-full", 200, "", 0, "", 0, -1, false, false},
		{"partial-full", 206, "bytes 0-3/16", 4, "0123", 0, -1, true, false},
		{"quota", 429, "", 0, "", 4, 8, true, false},
		{"forbidden", 403, "", 0, "", 4, 8, true, false},
		{"unauthorized", 401, "", 0, "", 4, 8, true, false},
		{"unsatisfied", 416, "bytes */16", 0, "", 16, 20, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &rangeContractBody{Reader: strings.NewReader(tc.body)}
			cfg := NewConfig()
			u, _ := url.Parse("https://registry.example.test")
			cfg.SetUrl(*u)
			cfg.SetHttpClient(&http.Client{Transport: rangeContractRT(func(r *http.Request) (*http.Response, error) {
				h := http.Header{}
				if tc.span != "" {
					h.Set("Content-Range", tc.span)
				}
				if tc.status == 429 {
					h.Set("Retry-After", "10")
				}
				return &http.Response{StatusCode: tc.status, Header: h, ContentLength: tc.length, Body: body, Request: r}, nil
			})})
			api, err := NewRegistryApi(cfg)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := api.RangeBlobs(context.Background(), NewRefspec("owner/image", "latest"), 2, "sha256:fixture", tc.start, tc.end, nil)
			if tc.wantErr {
				if err == nil {
					resp.Body.Close()
					t.Fatal("invalid response accepted")
				}
				if body.closes != 1 {
					t.Fatal("error body ownership", body.closes)
				}
				if tc.status == 429 {
					var quota interface {
						HTTPStatus() int
						RetryAfter() string
					}
					if !errors.As(err, &quota) || quota.HTTPStatus() != 429 || quota.RetryAfter() != "10" {
						t.Fatal("quota lost", err)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if body.closes != 0 {
				t.Fatal("closed successful body")
			}
			got := make([]byte, len(tc.body))
			if tc.short {
				got = make([]byte, tc.length)
			}
			_, err = io.ReadFull(resp.Body, got)
			resp.Body.Close()
			if tc.short {
				if !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatal("short body hidden", err)
				}
			} else if err != nil || string(got) != tc.body {
				t.Fatal(fmt.Sprint("wrong body ", err))
			}
			if body.closes != 1 {
				t.Fatal("successful body ownership", body.closes)
			}
		})
	}
}
