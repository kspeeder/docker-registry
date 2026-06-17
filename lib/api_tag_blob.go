package lib

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
		if shouldFallbackBlobInfoToRangeGet(resp.StatusCode) {
			size, modTime, header, fallbackErr := r.blobInfoWithRangeGet(ctx, ref.Repository(), url, headers)
			if fallbackErr == nil {
				return size, modTime, header, nil
			}
		}
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

func shouldFallbackBlobInfoToRangeGet(statusCode int) bool {
	switch statusCode {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return true
	default:
		return false
	}
}

func (r *registryApi) blobInfoWithRangeGet(ctx context.Context, repository string, url *url.URL, headers map[string]string) (int64, time.Time, http.Header, error) {
	rangeHeaders := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		rangeHeaders[k] = v
	}
	rangeHeaders["Range"] = "bytes=0-0"

	resp, err := r.getFollowingRedirects(ctx, url, rangeHeaders, cacheHintBlob(repository))
	if err != nil {
		return 0, time.Time{}, nil, err
	}
	defer resp.Body.Close()

	var size int64
	switch resp.StatusCode {
	case http.StatusPartialContent:
		size, err = parseContentRangeSize(resp.Header.Get("Content-Range"))
		if err != nil {
			return 0, time.Time{}, nil, err
		}
	case http.StatusOK:
		size, err = strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		if err != nil {
			return 0, time.Time{}, nil, fmt.Errorf("invalid Content-Length: %v", err)
		}
	default:
		return 0, time.Time{}, nil, fmt.Errorf("unexpected fallback status code: %d", resp.StatusCode)
	}

	lastModified, err := time.Parse(time.RFC1123, resp.Header.Get("Last-Modified"))
	if err != nil {
		return size, time.Time{}, resp.Header, nil
	}
	return size, lastModified, resp.Header, nil
}

func (r *registryApi) getFollowingRedirects(ctx context.Context, url *url.URL, headers map[string]string, hint string) (*http.Response, error) {
	currentURL := url
	currentHint := hint
	for redirects := 0; ; redirects++ {
		resp, err := r.connector.Get(ctx, currentURL, headers, currentHint)
		if err != nil {
			return nil, err
		}
		if !isRedirectStatus(resp.StatusCode) {
			return resp, nil
		}
		location := resp.Header.Get("Location")
		resp.Body.Close()
		if location == "" {
			return nil, fmt.Errorf("redirect missing Location")
		}
		if redirects >= 3 {
			return nil, fmt.Errorf("redirect limit exceeded")
		}
		nextURL, err := currentURL.Parse(location)
		if err != nil {
			return nil, fmt.Errorf("parse redirect Location: %w", err)
		}
		currentURL = nextURL
		currentHint = ""
	}
}

func isRedirectStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func parseContentRangeSize(contentRange string) (int64, error) {
	slash := strings.LastIndex(contentRange, "/")
	if slash == -1 || slash == len(contentRange)-1 {
		return 0, fmt.Errorf("invalid Content-Range: %s", contentRange)
	}
	total := strings.TrimSpace(contentRange[slash+1:])
	if total == "*" {
		return 0, fmt.Errorf("invalid Content-Range total: %s", contentRange)
	}
	size, err := strconv.ParseInt(total, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid Content-Range total: %v", err)
	}
	return size, nil
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

	apiResponse, err := r.getFollowingRedirects(
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
		return nil, invalidStatusCodeErrorFromResponse(apiResponse)
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
		apiResponse.Body.Close()
		return nil, genericAuthorizationError

	case http.StatusNotFound:
		apiResponse.Body.Close()
		return nil, newNotFoundError(fmt.Sprintf("blob %s not found in repository %s", digest, ref.Repository()))

	case http.StatusOK:
		return apiResponse.Body, nil

	default:
		err := invalidStatusCodeErrorFromResponse(apiResponse)
		apiResponse.Body.Close()
		return nil, err
	}
}
