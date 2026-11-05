package lib

import (
	"net/http"
	"strconv"
	"strings"
)

// ValidateBlobResponse checks the requested byte representation without reading
// or closing the body. The caller retains ownership on both success and failure.
// end is exclusive; -1 requests the remainder of the object.
func ValidateBlobResponse(resp *http.Response, start, end int64) error {
	if resp == nil {
		return genericMalformedResponseError
	}
	if start < 0 || end != -1 && end <= start {
		return newInvalidRequestError("invalid blob range")
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return genericAuthorizationError
	case http.StatusNotFound:
		return newNotFoundError("blob not found")
	case http.StatusOK, http.StatusPartialContent:
	default:
		return statusCodeErrorFromResponse(resp)
	}
	ranged := start > 0 || end > 0
	if !ranged {
		if resp.StatusCode != http.StatusOK {
			return MalformedResponseError("partial response to a full blob request")
		}
		return nil
	}
	if resp.StatusCode != http.StatusPartialContent {
		return MalformedResponseError("range request did not return a partial blob")
	}
	fields := strings.Fields(resp.Header.Get("Content-Range"))
	if len(fields) != 2 || fields[0] != "bytes" || len(resp.Header.Values("Content-Range")) != 1 {
		return MalformedResponseError("invalid blob Content-Range")
	}
	span, totalText, ok := strings.Cut(fields[1], "/")
	if !ok {
		return MalformedResponseError("invalid blob Content-Range")
	}
	firstText, lastText, ok := strings.Cut(span, "-")
	if !ok {
		return MalformedResponseError("invalid blob Content-Range")
	}
	first, err := blobRangeNumber(firstText)
	if err != nil {
		return err
	}
	last, err := blobRangeNumber(lastText)
	if err != nil || first != start || last < first {
		return MalformedResponseError("blob range offset does not match the request")
	}
	total := int64(-1)
	if totalText != "*" {
		total, err = blobRangeNumber(totalText)
		if err != nil || total <= last {
			return MalformedResponseError("invalid blob range total")
		}
	}
	if end > 0 && last != end-1 || end == -1 && (total < 0 || last != total-1) {
		return MalformedResponseError("blob range end does not match the request")
	}
	length := last - first + 1
	if length <= 0 {
		return MalformedResponseError("invalid blob range length")
	}
	if text := resp.Header.Get("Content-Length"); text != "" {
		n, err := blobRangeNumber(text)
		if err != nil || n != length {
			return MalformedResponseError("blob Content-Length does not match the range")
		}
	} else if resp.ContentLength > 0 && resp.ContentLength != length {
		return MalformedResponseError("blob Content-Length does not match the range")
	}
	return nil
}

func blobRangeNumber(text string) (int64, error) {
	if text == "" || strings.Trim(text, "0123456789") != "" {
		return 0, MalformedResponseError("invalid blob range number")
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, MalformedResponseError("blob range number outside supported bounds")
	}
	return n, nil
}
