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
