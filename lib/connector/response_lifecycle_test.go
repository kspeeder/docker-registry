package connector_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/kspeeder/docker-registry/lib"
	"github.com/kspeeder/docker-registry/lib/connector"
)

type lifecycleTransport func(*http.Request) (*http.Response, error)

func (f lifecycleTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type lifecycleBody struct {
	io.Reader
	closes int
}

func (b *lifecycleBody) Close() error { b.closes++; return nil }

func TestConnectorClosesDiscardedChallenges(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(map[bool]string{false: "stale-token-refresh", true: "invalid-challenge"}[malformed], func(t *testing.T) {
			var discarded []*lifecycleBody
			requests, tokens := 0, 0
			cfg := lib.NewConfig()
			cfg.SetHttpClient(&http.Client{Transport: lifecycleTransport(func(r *http.Request) (*http.Response, error) {
				status, payload := http.StatusUnauthorized, "challenge"
				header := http.Header{"Www-Authenticate": {`Bearer realm="https://auth.example/token",service="example",scope="repository:owner/image:pull"`}}
				if malformed {
					header.Set("Www-Authenticate", "invalid")
				}
				if r.URL.Host == "auth.example" {
					tokens++
					status, payload = 200, `{"token":"old"}`
					if tokens > 1 {
						payload = `{"token":"new"}`
					}
				} else {
					requests++
					if requests == 2 || r.Header.Get("Authorization") == "Bearer new" {
						status, payload = 200, "manifest"
					}
				}
				body := &lifecycleBody{Reader: strings.NewReader(payload)}
				if status == http.StatusUnauthorized {
					discarded = append(discarded, body)
				}
				return &http.Response{StatusCode: status, Header: header, Body: body, Request: r}, nil
			})})
			client := connector.NewTokenAuthConnector(&cfg)
			u, _ := url.Parse("https://example/v2/owner/image/manifests/fixed")
			for _, hint := range []string{"first", "second"} {
				resp, err := client.Get(context.Background(), u, nil, hint)
				if malformed {
					if err == nil {
						t.Fatal("invalid challenge accepted")
					}
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if resp.StatusCode != 200 {
					t.Fatalf("unexpected response %d", resp.StatusCode)
				}
				if resp.Body.(*lifecycleBody).closes != 0 {
					t.Fatal("returned response closed before caller consumed it")
				}
				resp.Body.Close()
			}
			if !malformed && (tokens != 2 || len(discarded) != 3) {
				t.Fatalf("stale refresh not exercised: %d tokens, %d challenges", tokens, len(discarded))
			}
			for _, body := range discarded {
				if body.closes != 1 {
					t.Errorf("discarded challenge closed %d times", body.closes)
				}
			}
		})
	}
}
