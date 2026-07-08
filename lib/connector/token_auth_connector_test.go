package connector

import "testing"

func TestGetAuthenticateAddsScopeForRepositoryPath(t *testing.T) {
	auth := `Bearer realm="https://ghcr.io/token",service="ghcr.io"`

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "two level repository path",
			path: "/v2/linkease/linkease/manifests/latest",
			want: `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:linkease/linkease:pull"`,
		},
		{
			name: "multi level repository path",
			path: "/v2/absmach/magistrala/alarms/manifests/latest",
			want: `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:absmach/magistrala/alarms:pull"`,
		},
		{
			name: "two level tag list path",
			path: "/v2/linkease/linkease/tags/list",
			want: `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:linkease/linkease:pull"`,
		},
		{
			name: "multi level tag list path",
			path: "/v2/absmach/magistrala/alarms/tags/list",
			want: `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:absmach/magistrala/alarms:pull"`,
		},
		{
			name: "tag path component inside repository name",
			path: "/v2/acme/tags/frontend/tags/list",
			want: `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:acme/tags/frontend:pull"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getAuthenticate(tt.path, auth); got != tt.want {
				t.Fatalf("getAuthenticate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetAuthenticateKeepsExistingScope(t *testing.T) {
	auth := `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:absmach/magistrala/alarms:pull"`
	got := getAuthenticate("/v2/absmach/magistrala/alarms/manifests/latest", auth)
	if got != auth {
		t.Fatalf("getAuthenticate() = %q, want %q", got, auth)
	}
}

func TestGetAuthenticateKeepsUnsupportedChallengesUnchanged(t *testing.T) {
	tests := []struct {
		name string
		path string
		auth string
	}{
		{
			name: "malformed bearer challenge",
			path: "/v2/linkease/linkease/tags/list",
			auth: `Bearer realm="https://ghcr.io/token"`,
		},
		{
			name: "basic challenge",
			path: "/v2/linkease/linkease/tags/list",
			auth: `Basic realm="registry"`,
		},
		{
			name: "non registry path",
			path: "/api/linkease/linkease/tags/list",
			auth: `Bearer realm="https://ghcr.io/token",service="ghcr.io"`,
		},
		{
			name: "unsupported v2 path",
			path: "/v2/linkease/linkease/referrers/sha256:abc",
			auth: `Bearer realm="https://ghcr.io/token",service="ghcr.io"`,
		},
		{
			name: "incomplete tag list path",
			path: "/v2/linkease/linkease/tags",
			auth: `Bearer realm="https://ghcr.io/token",service="ghcr.io"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getAuthenticate(tt.path, tt.auth); got != tt.auth {
				t.Fatalf("getAuthenticate() = %q, want %q", got, tt.auth)
			}
		})
	}
}
