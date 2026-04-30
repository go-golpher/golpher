package golpher

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
)

type Request struct {
	http   *http.Request
	body   *Body
	params map[string]string
}

type Body struct {
	bytes []byte
	error error
}

func (request *Request) Headers() map[string][]string {
	return request.http.Header
}

func (request *Request) Raw() *http.Request {
	return request.http
}

func (request *Request) Context() context.Context {
	return request.http.Context()
}

func (request *Request) SetContext(ctx context.Context) {
	request.http = request.http.WithContext(ctx)
}

func (request *Request) Param(name string) string {
	if request.params == nil {
		return ""
	}
	return request.params[name]
}

func (request *Request) Query(name string) string {
	return request.http.URL.Query().Get(name)
}

func (request *Request) NewError(status int, err string) error {
	return ErrorGolpher{Code: status, Message: err}
}

func (request *Request) Body() *Body {
	if request.body != nil {
		return request.body
	}
	data, err := io.ReadAll(request.http.Body)
	if err != nil {
		request.body = &Body{error: err}
		return request.body
	}
	request.body = &Body{bytes: data}
	return request.body
}

func (body *Body) Bytes() []byte {
	return body.bytes
}

func (body *Body) JSON(v interface{}) error {
	if body.error != nil {
		return body.error
	}
	return json.Unmarshal(body.Bytes(), v)
}

func (body *Body) XML(v interface{}) error {
	if body.error != nil {
		return body.error
	}
	return xml.Unmarshal(body.Bytes(), v)
}
