package connector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/kspeeder/docker-registry/lib/auth"
)

type tokenAuthConnector struct {
	cfg           Config
	httpClient    *http.Client
	authenticator auth.Authenticator
	semaphore     semaphore
	tokenCache    *tokenCache
	stat          *statistics
}

func (r *tokenAuthConnector) Delete(ctx context.Context, url *url.URL, headers map[string]string, hint string) (*http.Response, error) {
	return r.Request(ctx, http.MethodDelete, url, headers, hint)
}

func (r *tokenAuthConnector) Get(ctx context.Context, url *url.URL, headers map[string]string, hint string) (*http.Response, error) {
	return r.Request(ctx, http.MethodGet, url, headers, hint)
}

func (r *tokenAuthConnector) Head(ctx context.Context, url *url.URL, headers map[string]string, hint string) (*http.Response, error) {
	return r.Request(ctx, http.MethodHead, url, headers, hint)
}

func (r *tokenAuthConnector) Request(
	ctx context.Context,
	method string,
	url *url.URL,
	headers map[string]string,
	hint string,
) (response *http.Response, err error) {
	r.semaphore.Lock()
	defer r.semaphore.Unlock()

	r.stat.Request()

	var token auth.Token
	request, err := http.NewRequest(method, url.String(), strings.NewReader(""))
	if err != nil {
		return
	}

	if ctx != nil && ctx != context.TODO() && ctx != context.Background() {
		request = request.WithContext(ctx)
	}

	for header, value := range headers {
		request.Header.Set(header, value)
	}

	if hint != "" {
		if token = r.tokenCache.Get(hint); token != nil {
			r.stat.CacheHitAtApiLevel()
		} else {
			r.stat.CacheMissAtApiLevel()
		}
	}

	resp, err := r.attemptRequestWithToken(request, token)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		response = resp
		return
	}

	if token != nil {
		r.stat.CacheFailAtApiLevel()
	}

	if resp.Close {
		resp.Body.Close()
	}

	authenticate := getAuthenticate(request.URL.Path, resp.Header.Get("www-authenticate"))
	challenge, err := auth.ParseChallenge(authenticate)

	if err != nil {
		err = errors.New(err.Error() +
			" Are you shure that you are using the correct (token) auth scheme?")

		return
	}

	token, err = r.authenticator.Authenticate(challenge, false)

	if err != nil {
		return
	}

	if token != nil {
		if token.Fresh() {
			r.stat.CacheMissAtAuthLevel()
		} else {
			r.stat.CacheHitAtAuthLevel()
		}
	}

	response, err = r.attemptRequestWithToken(request, token)

	if err == nil &&
		response.StatusCode == http.StatusUnauthorized &&
		!token.Fresh() {

		r.stat.CacheFailAtAuthLevel()

		token, err = r.authenticator.Authenticate(challenge, true)

		if err != nil {
			return
		}

		response, err = r.attemptRequestWithToken(request, token)
	}

	if hint != "" && err == nil && response.StatusCode != http.StatusUnauthorized {
		r.tokenCache.Set(hint, token)
	}

	return
}

var challengeRegex2 *regexp.Regexp = regexp.MustCompile(
	`^\s*Bearer\s+realm="([^"]+)",service="([^"]+)"$`)

func getAuthenticate(reqUrl, auth string) string {
	// Www-Authenticate: Bearer realm="https://auth.m.daocloud.io/auth/token",service="docker.m.daocloud.io"
	// ,scope="repository:linkease/linkease:pull"
	// GET /v2/linkease/linkease/manifests/1.6.7 HTTP/1.1
	if strings.Contains(auth, "scope") {
		return auth
	}
	reqStrs := strings.SplitN(reqUrl, "/", 5)
	if len(reqStrs) < 5 {
		return auth
	}
	match := challengeRegex2.FindAllStringSubmatch(auth, -1)
	if len(match) == 1 {
		newAuth := fmt.Sprintf("%s,scope=\"repository:%s/%s:pull\"", auth, reqStrs[2], reqStrs[3])
		//fmt.Println("Change to", newAuth)
		return newAuth
	}
	return auth
}

func (r *tokenAuthConnector) attemptRequestWithToken(request *http.Request, token auth.Token) (*http.Response, error) {
	if token != nil {
		request.Header.Set("Authorization", "Bearer "+token.Value())
	}

	/* b, err2 := httputil.DumpRequest(request, true)
	if err2 == nil {
		fmt.Println(string(b)) comment
	} */

	resp, err := r.httpClient.Do(request)

	/* if err == nil {
		b, err2 := httputil.DumpResponse(resp, false)
		if err2 == nil {
			fmt.Println(string(b)) comment
		}
	} */

	return resp, err
}

func (r *tokenAuthConnector) GetStatistics() Statistics {
	return r.stat
}

func NewTokenAuthConnector(cfg Config) Connector {
	connector := tokenAuthConnector{
		cfg:        cfg,
		httpClient: cfg.HttpClient(),
		semaphore:  newSemaphore(cfg.MaxConcurrentRequests()),
		tokenCache: newTokenCache(),
		stat:       new(statistics),
	}
	if connector.httpClient == nil {
		connector.httpClient = createHttpClient(cfg)
	}

	connector.authenticator = auth.NewAuthenticator(
		connector.httpClient,
		cfg.Credentials(),
		cfg.FastChannel(),
		cfg.FastChannelTokenProvider(),
	)

	//setInvalidTokenForTest(&connector)

	return &connector
}

/* func setInvalidTokenForTest(connector *tokenAuthConnector) {
	connector.tokenCache.Set("pull:linkease/linkease", &testToken{
		value: "initial_token",
		fresh: true,
	})
}

type testToken struct {
	value string
	fresh bool
}

func (t *testToken) Value() string {
	return t.value
}

func (t *testToken) Fresh() bool {
	return t.fresh
} */
