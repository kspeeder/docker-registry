package connector

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

type basicAuthConnector struct {
	cfg        Config
	httpClient *http.Client
	semaphore  semaphore
	stat       *statistics
}

func (r *basicAuthConnector) Delete(ctx context.Context, url *url.URL, headers map[string]string, hint string) (*http.Response, error) {
	return r.Request(ctx, "DELETE", url, headers, hint)
}

func (r *basicAuthConnector) Get(ctx context.Context, url *url.URL, headers map[string]string, hint string) (*http.Response, error) {
	return r.Request(ctx, "GET", url, headers, hint)
}

func (r *basicAuthConnector) Head(ctx context.Context, url *url.URL, headers map[string]string, hint string) (*http.Response, error) {
	return r.Request(ctx, http.MethodHead, url, headers, hint)
}

func (r *basicAuthConnector) GetStatistics() Statistics {
	return r.stat
}

func (r *basicAuthConnector) Request(
	ctx context.Context,
	method string,
	url *url.URL,
	headers map[string]string,
	hint string,
) (response *http.Response, err error) {
	r.semaphore.Lock()
	defer r.semaphore.Unlock()

	r.stat.Request()

	request, err := http.NewRequest(method, url.String(), strings.NewReader(""))
	if err != nil {
		return
	}

	if ctx != nil && ctx != context.TODO() && ctx != context.Background() {
		request = request.WithContext(ctx)
	}

	credentials := r.cfg.Credentials()
	if credentials.Password() != "" || credentials.User() != "" {
		request.SetBasicAuth(credentials.User(), credentials.Password())
	}

	for header, value := range headers {
		request.Header.Set(header, value)
	}

	response, err = r.httpClient.Do(request)
	//fmt.Println("auth err=", err)

	return
}

func NewBasicAuthConnector(cfg Config) Connector {
	c := &basicAuthConnector{
		cfg:        cfg,
		httpClient: cfg.HttpClient(),
		semaphore:  newSemaphore(cfg.MaxConcurrentRequests()),
		stat:       new(statistics),
	}
	if c.httpClient == nil {
		c.httpClient = createHttpClient(cfg)
	}
	return c
}
