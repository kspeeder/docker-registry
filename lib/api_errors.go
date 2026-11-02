package lib

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

type AutorizationError string
type NotImplementedByRemoteError string
type MalformedResponseError string
type InvalidStatusCodeError string
type NotFoundError string
type InvalidRequestError string

// RateLimitError keeps quota metadata available through wrapped registry errors.
// Its message deliberately excludes the upstream response body.
type RateLimitError struct {
	retryAfter string
}

func (e RateLimitError) Error() string      { return e.Unwrap().Error() }
func (e RateLimitError) Unwrap() error      { return newInvalidStatusCodeError(http.StatusTooManyRequests) }
func (e RateLimitError) HTTPStatus() int    { return http.StatusTooManyRequests }
func (e RateLimitError) RetryAfter() string { return e.retryAfter }

var genericAuthorizationError AutorizationError = "authorization rejected by registry"
var genericMalformedResponseError MalformedResponseError = "malformed response"

// Preserve quota metadata through the error path without retaining a response
// body, credentials or an upstream error page. Existing classification still
// finds InvalidStatusCodeError through errors.As.
type rateLimitError struct{ retryAfter string }

func (e *rateLimitError) Error() string      { return "upstream rate limited" }
func (e *rateLimitError) HTTPStatus() int    { return http.StatusTooManyRequests }
func (e *rateLimitError) RetryAfter() string { return e.retryAfter }
func (e *rateLimitError) Unwrap() error      { return newInvalidStatusCodeError(http.StatusTooManyRequests) }

func (e AutorizationError) Error() string {
	return string(e)
}

func (e NotImplementedByRemoteError) Error() string {
	return string(e)
}

func (e MalformedResponseError) Error() string {
	return string(e)
}

func (e InvalidStatusCodeError) Error() string {
	return string(e)
}

func (e NotFoundError) Error() string {
	return string(e)
}

func (e InvalidRequestError) Error() string {
	return string(e)
}

func newInvalidStatusCodeError(code int) error {
	return InvalidStatusCodeError(fmt.Sprintf("invalid API response status %d", code))
}

func newNotFoundError(description string) error {
	return NotFoundError(description)
}

func newInvalidRequestError(description string) error {
	return InvalidRequestError(description)
}

func statusCodeErrorFromResponse(resp *http.Response) error {
	if resp.StatusCode == http.StatusTooManyRequests {
		return RateLimitError{retryAfter: resp.Header.Get("Retry-After")}
	}
	return newInvalidStatusCodeError(resp.StatusCode)
}

func invalidStatusCodeErrorFromResponse(resp *http.Response) error {
	if resp == nil {
		return InvalidStatusCodeError("invalid API response status")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return statusCodeErrorFromResponse(resp)
	}

	msg := fmt.Sprintf("invalid API response status %d", resp.StatusCode)

	if resp.Body != nil {
		if data, err := io.ReadAll(io.LimitReader(resp.Body, 2048)); err == nil {
			if trimmed := strings.TrimSpace(string(data)); trimmed != "" {
				msg = fmt.Sprintf("%s: %s", msg, trimmed)
			}
		}
	}

	return InvalidStatusCodeError(msg)
}
