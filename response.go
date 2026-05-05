package golpher

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

type Response struct {
	body               bytes.Buffer
	statusCode         int
	writer             http.ResponseWriter
	disableBodyCapture bool
}

func (response *Response) Status(code int) *Response {
	response.statusCode = code
	return response
}

func (response *Response) Raw() http.ResponseWriter {
	return response.writer
}

func (response *Response) Header() http.Header {
	return response.writer.Header()
}

func (response *Response) Send(body []byte) error {
	response.writeStatus()
	if !response.disableBodyCapture {
		response.body.Write(body)
	}
	_, err := response.writer.Write(body)
	return err
}

func (response *Response) Body() []byte {
	return response.body.Bytes()
}

func (response *Response) BodyString() string {
	return response.body.String()
}

func (response *Response) String(body string) error {
	response.writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	response.writeStatus()
	if !response.disableBodyCapture {
		response.body.WriteString(body)
	}
	_, err := io.WriteString(response.writer, body)
	return err
}

func (response *Response) JSON(obj interface{}) error {
	response.writer.Header().Set("Content-Type", "application/json")
	response.writeStatus()
	return json.NewEncoder(response.writer).Encode(obj)
}

func (response *Response) JSONBytes(body []byte) error {
	return response.Bytes(response.statusOrOK(), "application/json", body)
}

func (response *Response) Bytes(status int, contentType string, body []byte) error {
	if status == 0 {
		status = response.statusOrOK()
	}
	header := response.writer.Header()
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	header.Set("Content-Length", strconv.Itoa(len(body)))
	response.writer.WriteHeader(status)
	_, err := response.writer.Write(body)
	return err
}

func (response *Response) XML(obj interface{}) error {
	response.writer.Header().Set("Content-Type", "application/xml")
	response.writeStatus()
	return xml.NewEncoder(response.writer).Encode(obj)
}

func (response *Response) Redirect(url string, codes ...int) error {
	code := http.StatusFound
	if len(codes) > 0 {
		code = codes[0]
	}
	response.writer.Header().Set("Location", url)
	response.Status(code)
	return response.String(fmt.Sprintf("<a href=\"%s\">Found</a>.\n", url))
}

func (response *Response) writeStatus() {
	if response.statusCode != 0 {
		response.writer.WriteHeader(response.statusCode)
	}
}

func (response *Response) statusOrOK() int {
	if response.statusCode != 0 {
		return response.statusCode
	}
	return http.StatusOK
}
