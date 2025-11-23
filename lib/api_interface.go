package lib

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/kspeeder/docker-registry/lib/connector"
)

type Repository interface {
	Name() string
}

type RepositoryListResponse interface {
	Repositories() <-chan Repository
	LastError() error
}

type Tag interface {
	Name() string
	RepositoryName() string
}

type TagListResponse interface {
	Tags() <-chan Tag
	LastError() error
}

type LayerDetails interface {
	ContentDigest() string
}

type TagDetails interface {
	RawManifest() interface{}
	ContentDigest() string
	RepositoryName() string
	TagName() string
	Layers() []LayerDetails
}

type RegistryApi interface {
	ListRepositories() RepositoryListResponse
	ListTags(repositoryName string) TagListResponse
	GetTagDetails(ctx context.Context, ref Refspec, manifestVersion uint) (TagDetails, error)
	DeleteTag(ref Refspec) error
	GetStatistics() connector.Statistics
	GetBlobs(ctx context.Context, ref Refspec, manifestVersion uint, digest string) (io.ReadCloser, error)
	BlobInfo(ctx context.Context, ref Refspec, manifestVersion uint, digest string, extraHeaders map[string]string) (int64, time.Time, http.Header, error)
	RangeBlobs(ctx context.Context, ref Refspec, manifestVersion uint, digest string, start, end int64, extraHeaders map[string]string) (*http.Response, error)
	Manifests(ctx context.Context, head bool, ref Refspec, manifestVersion uint, extraHeaders map[string]string) (*http.Response, error)
}
