package auth

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type lifecycleTransport func(*http.Request) (*http.Response, error)

func (f lifecycleTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type lifecycleBody struct {
	io.Reader
	closes int
}

func (b *lifecycleBody) Close() error { b.closes++; return nil }

type lifecycleCredentials struct{ refresh string }

func (c lifecycleCredentials) User() string          { return "" }
func (c lifecycleCredentials) Password() string      { return "" }
func (c lifecycleCredentials) IdentityToken() string { return c.refresh }
func (c lifecycleCredentials) SetUser(string)        {}
func (c lifecycleCredentials) SetPassword(string)    {}

func TestAuthenticatorClosesConsumedResponses(t *testing.T) {
	for _, oauth := range []bool{false, true} {
		for _, outcome := range []string{"success", "status-error", "decode-error"} {
			name := "jwt/" + outcome
			credentials := lifecycleCredentials{}
			payload := `{"token":"fixture"}`
			if oauth {
				name = "oauth/" + outcome
				credentials.refresh = "fixture-refresh"
				payload = `{"access_token":"fixture"}`
			}
			t.Run(name, func(t *testing.T) {
				status := http.StatusOK
				if outcome == "status-error" {
					status = http.StatusForbidden
				}
				if outcome == "decode-error" {
					payload = `{`
				}
				body := &lifecycleBody{Reader: strings.NewReader(payload)}
				client := &http.Client{Transport: lifecycleTransport(func(r *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: body, Request: r}, nil
				})}
				challenge, err := ParseChallenge(`Bearer realm="https://auth.example/token",service="example",scope="repository:owner/image:pull"`)
				if err != nil {
					t.Fatal(err)
				}
				token, err := NewAuthenticator(client, credentials, false, nil).Authenticate(challenge, false)
				if (err == nil) != (outcome == "success") {
					t.Fatalf("unexpected authentication result: %v", err)
				}
				if err == nil && token.Value() != "fixture" {
					t.Fatal("wrong token")
				}
				if body.closes != 1 {
					t.Fatalf("consumed response closed %d times", body.closes)
				}
			})
		}
	}
}
