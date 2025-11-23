package connector

import (
	"context"
	"net/http"
	"net/url"
)

type Connector interface {
	Request(ctx context.Context, method string, url *url.URL, headers map[string]string, hint string) (*http.Response, error)
	Delete(ctx context.Context, url *url.URL, headers map[string]string, hint string) (*http.Response, error)
	Get(ctx context.Context, url *url.URL, headers map[string]string, hint string) (*http.Response, error)
	Head(ctx context.Context, url *url.URL, headers map[string]string, hint string) (*http.Response, error)
	GetStatistics() Statistics
}
