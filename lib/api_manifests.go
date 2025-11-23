package lib

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

func (r *registryApi) Manifests(ctx context.Context, head bool, ref Refspec, manifestVersion uint, extraHeaders map[string]string) (*http.Response, error) {
	// Implementation of Manifests method
	if ref.Repository() == "" || ref.Reference() == "" {
		return nil, errors.New("invalid parameters: repository and reference must be non-empty")
	}

	url := r.endpointUrl(fmt.Sprintf("v2/%s/manifests/%s", ref.Repository(), ref.Reference()))
	headers, err := r.getHeadersForManifestVersion(manifestVersion) // Use manifest v2 headers
	if err != nil {
		return nil, err
	}
	if len(extraHeaders) > 0 {
		for k, v := range extraHeaders {
			headers[k] = v
		}
	}
	var apiResponse *http.Response
	if head {
		apiResponse, err = r.connector.Head(
			ctx,
			url,
			headers,
			cacheHintBlob(ref.Repository()),
		)
	} else {
		apiResponse, err = r.connector.Get(
			ctx,
			url,
			headers,
			cacheHintBlob(ref.Repository()),
		)
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		if apiResponse != nil {
			apiResponse.Body.Close()
		}
	}()

	switch apiResponse.StatusCode {
	case http.StatusForbidden, http.StatusUnauthorized:
		return nil, genericAuthorizationError

	case http.StatusNotFound:
		return nil, newNotFoundError(fmt.Sprintf("manifest %s not found in repository %s", ref.Reference(), ref.Repository()))
	case http.StatusOK:
		respCopy := apiResponse
		apiResponse = nil
		return respCopy, nil
	default:
		return nil, newInvalidStatusCodeError(apiResponse.StatusCode)
	}
}
