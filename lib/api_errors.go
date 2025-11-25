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

var genericAuthorizationError AutorizationError = "authorization rejected by registry"
var genericMalformedResponseError MalformedResponseError = "malformed response"

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

func invalidStatusCodeErrorFromResponse(resp *http.Response) error {
	if resp == nil {
		return InvalidStatusCodeError("invalid API response status")
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
