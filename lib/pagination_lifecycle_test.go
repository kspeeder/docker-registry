package lib_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kspeeder/docker-registry/lib"
)

type pageTransport func(*http.Request) (*http.Response, error)

func (f pageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type pageBody struct {
	io.Reader
	closes int
}

func (b *pageBody) Close() error { b.closes++; return nil }

func TestPaginationClosesEveryConsumedResponse(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		status, pages int
		wantError     bool
	}{
		{"two-pages", `{"repositories":["owner/image"]}`, 200, 2, false},
		{"denied", `denied`, 403, 1, true},
		{"broken-json", `{`, 200, 1, true},
		{"invalid-document", `{}`, 200, 1, true},
	} {
		for _, connectionClose := range []bool{false, true} {
			name := tc.name + "/keep-alive"
			if connectionClose {
				name = tc.name + "/connection-close"
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv("DOCKER_CONFIG", t.TempDir())
				var bodies []*pageBody
				cfg := lib.NewConfig()
				cfg.SetUseBasicAuth(true)
				cfg.SetHttpClient(&http.Client{Transport: pageTransport(func(req *http.Request) (*http.Response, error) {
					body := &pageBody{Reader: strings.NewReader(tc.payload)}
					bodies = append(bodies, body)
					h := make(http.Header)
					if len(bodies) < tc.pages {
						h.Set("Link", `</v2/_catalog?last=owner/image>; rel="next"`)
					}
					return &http.Response{StatusCode: tc.status, Header: h, Body: body, Close: connectionClose, Request: req}, nil
				})})
				api, err := lib.NewRegistryApi(cfg)
				if err != nil {
					t.Fatal(err)
				}
				response := api.ListRepositories()
				count := 0
				for range response.Repositories() {
					count++
				}
				if (response.LastError() != nil) != tc.wantError {
					t.Fatalf("unexpected error: %v", response.LastError())
				}
				if len(bodies) != tc.pages {
					t.Fatalf("pages=%d want=%d", len(bodies), tc.pages)
				}
				if !tc.wantError && count != tc.pages {
					t.Fatalf("repositories=%d want=%d", count, tc.pages)
				}
				for i, b := range bodies {
					if b.closes != 1 {
						t.Errorf("page %d closed %d times; Response.Close=%v", i, b.closes, connectionClose)
					}
				}
			})
		}
	}
}
