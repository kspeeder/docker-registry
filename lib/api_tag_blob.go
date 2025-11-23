package lib

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

func (r *registryApi) BlobInfo(ctx context.Context, ref Refspec, manifestVersion uint, digest string, extraHeaders map[string]string) (int64, time.Time, http.Header, error) {
	// Implementation of GetBlobs method
	if ref.Repository() == "" || ref.Reference() == "" || digest == "" {
		return 0, time.Time{}, nil, errors.New("invalid parameters: repository, reference and digest must be non-empty")
	}

	url := r.endpointUrl(fmt.Sprintf("v2/%s/blobs/%s", ref.Repository(), digest))
	headers, err := r.getHeadersForManifestVersion(manifestVersion) // Use manifest v2 headers
	if err != nil {
		return 0, time.Time{}, nil, err
	}
	if len(extraHeaders) > 0 {
		for k, v := range extraHeaders {
			headers[k] = v
		}
	}

	resp, err := r.connector.Head(
		ctx,
		url,
		headers,
		cacheHintBlob(ref.Repository()),
	)
	if err != nil {
		return 0, time.Time{}, nil, err
	}
	defer resp.Body.Close()

	/* if resp != nil {
		b, err := httputil.DumpResponse(resp, false)
		if err == nil {
			fmt.Println("blobs=\n", string(b)) comment
		}
	} */

	if resp.StatusCode != http.StatusOK {
		return 0, time.Time{}, nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	contentLength := resp.Header.Get("Content-Length")
	if contentLength == "" {
		return 0, time.Time{}, nil, fmt.Errorf("missing Content-Length header")
	}

	size, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		return 0, time.Time{}, nil, fmt.Errorf("invalid Content-Length: %v", err)
	}

	//fmt.Println("last-modified=", resp.Header.Get("Last-Modified"))
	lastModified, err := time.Parse(time.RFC1123, resp.Header.Get("Last-Modified"))
	if err != nil {
		return size, time.Time{}, resp.Header, nil // Return size even if last modified time is unavailable
	}

	return size, lastModified, resp.Header, nil
}

func (r *registryApi) RangeBlobs(ctx context.Context, ref Refspec, manifestVersion uint, digest string, start, end int64, extraHeaders map[string]string) (*http.Response, error) {
	// Implementation of GetBlobs method
	if ref.Repository() == "" || ref.Reference() == "" || digest == "" || (end != -1 && start >= end) {
		return nil, errors.New("invalid parameters: repository, reference and digest must be non-empty")
	}

	url := r.endpointUrl(fmt.Sprintf("v2/%s/blobs/%s", ref.Repository(), digest))
	headers, err := r.getHeadersForManifestVersion(manifestVersion) // Use manifest v2 headers
	if err != nil {
		return nil, err
	}
	// Set Range header in format "bytes=start-end"
	var useRange bool
	if end > 0 {
		headers["Range"] = fmt.Sprintf("bytes=%d-%d", start, end-1)
		useRange = true
	}
	if len(extraHeaders) > 0 {
		for k, v := range extraHeaders {
			headers[k] = v
		}
	}

	apiResponse, err := r.connector.Get(
		ctx,
		url,
		headers,
		cacheHintBlob(ref.Repository()),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if apiResponse != nil {
			apiResponse.Body.Close()
		}
	}()

	/* if apiResponse != nil {
		b, err := httputil.DumpResponse(apiResponse, false)
		if err == nil {
			fmt.Println("blobs=\n", string(b)) comment
		}
	} */

	switch apiResponse.StatusCode {
	case http.StatusForbidden, http.StatusUnauthorized:
		if apiResponse.Close {
			apiResponse.Body.Close()
		}
		return nil, genericAuthorizationError

	case http.StatusNotFound:
		if apiResponse.Close {
			apiResponse.Body.Close()
		}
		return nil, newNotFoundError(fmt.Sprintf("blob %s not found in repository %s", digest, ref.Repository()))
	case http.StatusOK:
		// Got 200 response with range request, should be 206
		if useRange {
			apiResponse.Body.Close()
			return nil, errors.New("got 200 response with range request")
		}
		respCopy := apiResponse
		apiResponse = nil
		return respCopy, nil
	case http.StatusPartialContent:
		respCopy := apiResponse
		apiResponse = nil
		return respCopy, nil
	default:
		return nil, newInvalidStatusCodeError(apiResponse.StatusCode)
	}
}

func (r *registryApi) GetBlobs(ctx context.Context, ref Refspec, manifestVersion uint, digest string) (io.ReadCloser, error) {
	// Implementation of GetBlobs method
	if ref.Repository() == "" || ref.Reference() == "" || digest == "" {
		return nil, errors.New("invalid parameters: repository, reference and digest must be non-empty")
	}

	url := r.endpointUrl(fmt.Sprintf("v2/%s/blobs/%s", ref.Repository(), digest))
	headers, err := r.getHeadersForManifestVersion(manifestVersion) // Use manifest v2 headers
	if err != nil {
		return nil, err
	}

	apiResponse, err := r.connector.Get(
		ctx,
		url,
		headers,
		cacheHintBlob(ref.Repository()),
	)
	if err != nil {
		return nil, err
	}

	/* if apiResponse != nil {
		b, err := httputil.DumpResponse(apiResponse, false)
		if err == nil {
			fmt.Println("blobs=\n", string(b))  comment
		}
	} */

	switch apiResponse.StatusCode {
	case http.StatusForbidden, http.StatusUnauthorized:
		if apiResponse.Close {
			apiResponse.Body.Close()
		}
		return nil, genericAuthorizationError

	case http.StatusNotFound:
		if apiResponse.Close {
			apiResponse.Body.Close()
		}
		return nil, newNotFoundError(fmt.Sprintf("blob %s not found in repository %s", digest, ref.Repository()))

	case http.StatusOK:
		return apiResponse.Body, nil

	default:
		if apiResponse.Close {
			apiResponse.Body.Close()
		}
		return nil, newInvalidStatusCodeError(apiResponse.StatusCode)
	}
}
