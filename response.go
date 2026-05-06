package golpher

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"unsafe"
)

type Response struct {
	body               bytes.Buffer
	statusCode         int
	writer             http.ResponseWriter
	disableBodyCapture bool
}

var (
	textPlainCharsetUTF8Header   = []string{"text/plain; charset=utf-8"}
	textPlainHeader              = []string{"text/plain"}
	applicationJSONHeader        = []string{"application/json"}
	applicationOctetStreamHeader = []string{"application/octet-stream"}
	commonContentLengthHeaders   [128][]string
)

func init() {
	for i := range commonContentLengthHeaders {
		commonContentLengthHeaders[i] = []string{itoa(i)}
	}
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
	response.writer.Header()["Content-Type"] = textPlainCharsetUTF8Header
	response.writeStatus()
	if !response.disableBodyCapture {
		response.body.WriteString(body)
	}
	_, err := writeString(response.writer, body)
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
		header["Content-Type"] = contentTypeHeader(contentType)
	}
	header["Content-Length"] = contentLengthHeader(len(body))
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

type stringWriter interface {
	WriteString(string) (int, error)
}

func writeString(writer http.ResponseWriter, body string) (int, error) {
	if writer, ok := writer.(stringWriter); ok {
		return writer.WriteString(body)
	}
	return writer.Write(unsafeStringBytes(body))
}

func unsafeStringBytes(body string) []byte {
	if body == "" {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(body), len(body))
}

func contentTypeHeader(contentType string) []string {
	switch contentType {
	case "application/json":
		return applicationJSONHeader
	case "text/plain":
		return textPlainHeader
	case "text/plain; charset=utf-8":
		return textPlainCharsetUTF8Header
	case "application/octet-stream":
		return applicationOctetStreamHeader
	default:
		return []string{contentType}
	}
}

func contentLengthHeader(length int) []string {
	if length >= 0 && length < len(commonContentLengthHeaders) {
		return commonContentLengthHeaders[length]
	}
	return []string{itoa(length)}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
