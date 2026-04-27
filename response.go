package golpher

import (
	"encoding/json"
	"encoding/xml"
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

func (response *Response) JSON(obj interface{}) error {
	response.writer.Header().Set("Content-Type", "application/json")
	if response.statusCode != 0 {
		response.writer.WriteHeader(response.statusCode)
	}
	return json.NewEncoder(response.writer).Encode(obj)
}

func (response *Response) XML(obj interface{}) error {
	response.writer.Header().Set("Content-Type", "application/xml")
	if response.statusCode != 0 {
		response.writer.WriteHeader(response.statusCode)
	}
	return xml.NewEncoder(response.writer).Encode(obj)
}
