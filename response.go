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

// Response wraps an http.ResponseWriter. It tracks whether the
// response has been committed, the effective status code, and the
// number of bytes written — including writes performed by net/http
// adapters. Body capture is opt-in via AppConfig.EnableResponseBodyCapture.
//
// After the first WriteHeader or Write the response is committed;
// further SetStatus calls are no-ops on the wire, but body writes
// remain possible.
type Response struct {
	writer         http.ResponseWriter // the tracking wrapper, not the original
	status         int                 // 0 = implicit 200 until committed
	bytesWritten   int64
	committed      bool
	captureEnabled bool
	body           bytes.Buffer // populated only when captureEnabled is true
}

// Committed reports whether the response status has been written to the wire.
func (res *Response) Committed() bool { return res.committed }

// Status returns the HTTP status code. If the response has not been committed
// and SetStatus was called, the staged code is returned. If neither was done,
// 0 is returned. Once committed without an explicit status, 200 is reported.
func (res *Response) Status() int {
	if res.status != 0 {
		return res.status
	}
	if res.committed {
		return http.StatusOK
	}
	return 0
}

// BytesWritten returns the total number of bytes successfully written to the
// wire through this Response.
func (res *Response) BytesWritten() int64 { return res.bytesWritten }

// SetStatus stages the HTTP status code. It has no effect after the response
// has been committed. Returns the Response for chaining.
func (res *Response) SetStatus(code int) *Response {
	if !res.committed {
		res.status = code
	}
	return res
}

// Raw returns the tracking http.ResponseWriter so that writes through any
// path (helpers, adapters, handler-owned code) are counted and tracked.
func (res *Response) Raw() http.ResponseWriter {
	return res.writer
}

// Header returns the response header map.
func (res *Response) Header() http.Header {
	return res.writer.Header()
}

// Body returns the captured response body. Returns nil when capture is
// disabled or nothing has been written.
func (res *Response) Body() []byte {
	if !res.captureEnabled {
		return nil
	}
	return res.body.Bytes()
}

// BodyString returns the captured response body as a string.
func (res *Response) BodyString() string {
	if !res.captureEnabled {
		return ""
	}
	return res.body.String()
}

// Send writes the raw byte slice. Capture happens in the tracking
// writer when enabled.
func (res *Response) Send(body []byte) error {
	res.writeStatus()
	_, err := res.writer.Write(body)
	return err
}

// String writes a plain-text response with Content-Type: text/plain; charset=utf-8.
func (res *Response) String(body string) error {
	res.Header().Set("Content-Type", "text/plain; charset=utf-8")
	res.writeStatus()
	_, err := io.WriteString(res.writer, body)
	return err
}

// JSON encodes obj as JSON with Content-Type: application/json.
func (res *Response) JSON(obj any) error {
	res.Header().Set("Content-Type", "application/json")
	res.writeStatus()
	return json.NewEncoder(res.writer).Encode(obj)
}

// JSONBytes writes a pre-encoded JSON body with Content-Type: application/json.
func (res *Response) JSONBytes(body []byte) error {
	return res.Bytes(res.statusOrOK(), "application/json", body)
}

// Bytes writes a raw body with explicit status, content type, and Content-Length
// set via Header().Set (safe under concurrent use).
func (res *Response) Bytes(status int, contentType string, body []byte) error {
	if status == 0 {
		status = res.statusOrOK()
	}
	header := res.writer.Header()
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	header.Set("Content-Length", strconv.Itoa(len(body)))
	res.SetStatus(status)
	res.writeStatus()
	_, err := res.writer.Write(body)
	return err
}

// XML encodes obj as XML with Content-Type: application/xml.
func (res *Response) XML(obj any) error {
	res.Header().Set("Content-Type", "application/xml")
	res.writeStatus()
	return xml.NewEncoder(res.writer).Encode(obj)
}

// Redirect sends an HTTP redirect. The default status is 302 Found.
func (res *Response) Redirect(url string, codes ...int) error {
	code := http.StatusFound
	if len(codes) > 0 {
		code = codes[0]
	}
	res.Header().Set("Location", url)
	res.SetStatus(code)
	return res.String(fmt.Sprintf(`<a href="%s">Found</a>.`+"\n", url))
}

// writeStatus flushes the staged status to the wire. On the first call with
// status == 0 the wrapper records implicit 200. Subsequent calls are no-ops
// on the wire because the wrapper prevents duplicate WriteHeader.
func (res *Response) writeStatus() {
	if res.committed {
		return
	}
	res.writer.WriteHeader(res.statusOrOK())
}

func (res *Response) statusOrOK() int {
	if res.status != 0 {
		return res.status
	}
	return http.StatusOK
}

// trackingResponseWriter wraps an http.ResponseWriter to record the
// committed status, write count, and (optionally) capture the body.
//
// Optional writer capabilities (Flush, Hijack, etc.) are reached
// through http.ResponseController, which traverses Unwrap.
type trackingResponseWriter struct {
	w           http.ResponseWriter
	res         *Response
	wroteHeader bool
}

func (w *trackingResponseWriter) Header() http.Header {
	return w.w.Header()
}

func (w *trackingResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.res.status = status
	w.res.committed = true
	w.w.WriteHeader(status)
}

func (w *trackingResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.w.Write(p)
	if n > 0 {
		w.res.bytesWritten += int64(n)
		if w.res.captureEnabled {
			w.res.body.Write(p[:n])
		}
	}
	return n, err
}

// WriteByte implements io.ByteWriter for efficiency when the underlying
// writer supports it. Falls back to Write([]byte{c}) with no double count.
func (w *trackingResponseWriter) WriteByte(c byte) error {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if bw, ok := w.w.(io.ByteWriter); ok {
		if err := bw.WriteByte(c); err != nil {
			return err
		}
		w.res.bytesWritten++
		if w.res.captureEnabled {
			w.res.body.WriteByte(c)
		}
		return nil
	}
	_, err := w.Write([]byte{c})
	return err
}

// WriteString implements io.StringWriter for efficiency.
func (w *trackingResponseWriter) WriteString(s string) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if sw, ok := w.w.(io.StringWriter); ok {
		n, err := sw.WriteString(s)
		if n > 0 {
			w.res.bytesWritten += int64(n)
			if w.res.captureEnabled {
				w.res.body.WriteString(s[:n])
			}
		}
		return n, err
	}
	return w.Write([]byte(s))
}

// Unwrap makes the underlying writer discoverable via
// http.ResponseController. Optional capabilities (Flush, Hijack, etc.)
// are reached through the controller, which traverses Unwrap.
func (w *trackingResponseWriter) Unwrap() http.ResponseWriter {
	return w.w
}
