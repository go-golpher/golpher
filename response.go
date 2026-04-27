package golpher

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
)

type Response struct {
	body       []byte
	statusCode int
	writer     http.ResponseWriter
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
	_, err := response.writer.Write(body)
	return err
}

func (response *Response) String(body string) error {
	response.writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	return response.Send([]byte(body))
}

func (response *Response) JSON(obj interface{}) error {
	response.writer.Header().Set("Content-Type", "application/json")
	response.writeStatus()
	return json.NewEncoder(response.writer).Encode(obj)
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
