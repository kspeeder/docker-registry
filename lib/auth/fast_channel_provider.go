package auth

// FastChannelTokenProvider fetches tokens for fast channel auth flows.
type FastChannelTokenProvider interface {
	RequestMirrorToken(registryHost string, service string, scope []string) (string, error)
}
