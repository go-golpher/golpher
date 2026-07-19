package golpher

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
)

// Request wraps an *http.Request with route parameters and body helpers.
type Request struct {
	http        *http.Request
	params      map[string]string
	paramNames  []string
	paramValues []string

	// response is set during request acquisition and used to
	// apply MaxBytesReader lazily.
	response *Response

	// bodyState caches the full body read result.
	bodyData    []byte
	bodyErr     error
	bodyRead    bool
	bodyWrapped bool

	// appBodyLimit is the app-level MaxRequestBodyBytes default.
	appBodyLimit int64
	// bodyLimitOverride is the per-request body limit set by
	// BodyLimit middleware. 0 means use the app-level default.
	bodyLimitOverride int64
}

// Headers returns the request headers.
func (r *Request) Headers() map[string][]string {
	return r.http.Header
}

// Raw returns the underlying *http.Request.
func (r *Request) Raw() *http.Request {
	return r.http
}

// Context returns the request's context.
func (r *Request) Context() context.Context {
	return r.http.Context()
}

// SetContext replaces the request's context.
func (r *Request) SetContext(ctx context.Context) {
	r.http = r.http.WithContext(ctx)
}

// Param returns the value of a named route parameter.
func (r *Request) Param(name string) string {
	if r.params == nil {
		for i, paramName := range r.paramNames {
			if paramName == name {
				return r.paramValues[i]
			}
		}
		return ""
	}
	return r.params[name]
}

// Query returns the value of a named URI query parameter.
func (r *Request) Query(name string) string {
	return r.http.URL.Query().Get(name)
}

// Body reads the full request body (at most once) and returns the
// bytes and any error. The body is bounded by the configured
// MaxRequestBodyBytes via a lazy http.MaxBytesReader.
//
// The read result is cached; subsequent calls return the same pair
// without re-reading. When the configured limit is exceeded the
// error satisfies errors.As(err, &maxBytesErr) for
// *http.MaxBytesError.
func (r *Request) Body() ([]byte, error) {
	if r.bodyRead {
		return r.bodyData, r.bodyErr
	}
	r.bodyRead = true

	body := r.body()
	if body == nil {
		r.bodyErr = io.EOF
		return nil, r.bodyErr
	}

	data, err := io.ReadAll(body)
	r.bodyData = data
	r.bodyErr = err
	return r.bodyData, r.bodyErr
}

// BodyJSON reads the request body (at most once) and unmarshals it
// as JSON into v.
func (r *Request) BodyJSON(v any) error {
	data, err := r.Body()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// BodyXML reads the request body (at most once) and unmarshals it
// as XML into v.
func (r *Request) BodyXML(v any) error {
	data, err := r.Body()
	if err != nil {
		return err
	}
	return xml.Unmarshal(data, v)
}

// body returns the underlying body, wrapped with http.MaxBytesReader
// the first time a positive limit is configured. Wrapping is
// idempotent: calling body() twice with the same limit wraps once.
// A nil response is tolerated; in that case no wrapping occurs.
func (r *Request) body() io.ReadCloser {
	if r.http.Body == nil {
		return nil
	}
	limit := r.effectiveBodyLimit()
	if limit < 0 {
		return r.http.Body
	}
	if limit > 0 && !r.bodyWrapped {
		r.bodyWrapped = true
		var w http.ResponseWriter
		if r.response != nil {
			w = r.response.Raw()
		}
		r.http.Body = http.MaxBytesReader(w, r.http.Body, limit)
	}
	return r.http.Body
}

// effectiveBodyLimit returns the per-request body limit:
//
//	bodyLimitOverride < 0  → unlimited
//	bodyLimitOverride > 0  → enforce that many bytes
//	bodyLimitOverride == 0 → use appBodyLimit
func (r *Request) effectiveBodyLimit() int64 {
	if r.bodyLimitOverride < 0 {
		return -1
	}
	if r.bodyLimitOverride > 0 {
		return r.bodyLimitOverride
	}
	return r.appBodyLimit
}
