package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type authenticator struct {
	httpClient    *http.Client
	credentials   RegistryCredentials
	cache         *tokenCache
	fastChannel   bool
	tokenProvider FastChannelTokenProvider
}

func (a *authenticator) Authenticate(c *Challenge, ignoreCached bool) (t Token, err error) {
	if !ignoreCached {
		value := a.cache.Get(c)

		if value != "" {
			t = newToken(value, false)
			return
		}
	}

	// Try OAuth2 authentication first, and then legacy JWT tokens.
	var decodedResponse authResponse
	if a.fastChannel {
		decodedResponse, err = a.fetchFastChannelToken(c)
	} else if refreshToken := a.credentials.IdentityToken(); refreshToken != "" {
		decodedResponse, err = a.fetchTokenOAuth2(c, refreshToken)
	} else {
		decodedResponse, err = a.fetchTokenJWT(c)
	}
	if err != nil {
		return
	}

	a.cache.Set(c, decodedResponse)
	t = newToken(decodedResponse.Token, true)

	return
}

func (a *authenticator) fetchFastChannelToken(challenge *Challenge) (decodedResponse authResponse, err error) {
	realm := challenge.Realm()
	service := challenge.Service()
	scope := challenge.Scope()

	if a.tokenProvider == nil {
		err = errors.New("fast channel token provider is not configured")
		return
	}

	mirrorToken, err := a.tokenProvider.RequestMirrorToken(realm.Host, service, scope)
	if err != nil {
		return
	}

	tokenData := map[string]string{
		"token": mirrorToken,
	}

	jsonData, err := json.Marshal(tokenData)
	if err != nil {
		return
	}

	decodedResponse, err = decodeAuthResponse(bytes.NewReader(jsonData))
	return
}

func (a *authenticator) fetchTokenJWT(c *Challenge) (decodedResponse authResponse, err error) {
	requestUrl := c.buildRequestUrl()
	authRequest, err := http.NewRequest("GET", requestUrl.String(), strings.NewReader(""))
	if err != nil {
		return
	}

	username := a.credentials.User()
	password := a.credentials.Password()
	if username != "" || password != "" {
		authRequest.SetBasicAuth(username, password)
	}

	authResponse, err := a.httpClient.Do(authRequest)
	if err != nil {
		return
	}

	if authResponse.Close {
		defer authResponse.Body.Close()
	}

	if authResponse.StatusCode != http.StatusOK {
		err = errors.New(fmt.Sprintf("authentication against auth server failed with code %d", authResponse.StatusCode))
		return
	}

	decodedResponse, err = decodeAuthResponse(authResponse.Body)
	return
}

func (a *authenticator) fetchTokenOAuth2(c *Challenge, refreshToken string) (decodedResponse authResponse, err error) {
	authResponse, err := a.httpClient.PostForm(c.realm.String(), url.Values{
		"grant_type":    {"refresh_token"},
		"service":       {c.service},
		"client_id":     {"docker-ls"},
		"scope":         {strings.Join(c.scope, " ")},
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return
	}

	if authResponse.StatusCode != http.StatusOK {
		err = errors.New(fmt.Sprintf("OAuth2 authentication against auth server failed with code %d", authResponse.StatusCode))
		return
	}

	decodedResponse, err = decodeAuth2Response(authResponse.Body)
	return
}

func NewAuthenticator(client *http.Client, credentials RegistryCredentials, fastChannel bool, tokenProvider FastChannelTokenProvider) Authenticator {
	auth := &authenticator{
		httpClient:    client,
		credentials:   credentials,
		cache:         newTokenCache(),
		fastChannel:   fastChannel,
		tokenProvider: tokenProvider,
	}
	return auth
}
