package connector

import (
	"net/http"

	"github.com/kspeeder/docker-registry/lib/auth"
)

type Config interface {
	MaxConcurrentRequests() uint
	Credentials() auth.RegistryCredentials
	AllowInsecure() bool
	UserAgent() string
	HttpClient() *http.Client
}
