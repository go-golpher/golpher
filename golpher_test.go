package golpher

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppImplementsHTTPHandler(t *testing.T) {
	var _ http.Handler = New()
}

func TestRouterGETDispatchesMatchingHandler(t *testing.T) {
	app := New()
	app.Router.GET("/hello", func(_ *Request, res *Response) error {
		return res.Status(http.StatusCreated).JSON(map[string]string{"message": "ok"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON body: %v", err)
	}
	if payload["message"] != "ok" {
		t.Fatalf("expected handler response, got %#v", payload)
	}
}

func TestRouterUnknownPathReturnsNotFound(t *testing.T) {
	app := New()
	app.Router.GET("/known", func(_ *Request, res *Response) error {
		return res.Status(http.StatusOK).JSON(map[string]string{"message": "known"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestRouterMethodMismatchReturnsMethodNotAllowed(t *testing.T) {
	app := New()
	app.Router.GET("/resource", func(_ *Request, res *Response) error {
		return res.Status(http.StatusOK).JSON(map[string]string{"message": "ok"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/resource", nil)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestResponseStatusThenJSONWritesStatusAndContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	res := &Response{writer: rec}

	if err := res.Status(http.StatusAccepted).JSON(map[string]string{"status": "accepted"}); err != nil {
		t.Fatalf("unexpected JSON error: %v", err)
	}

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", got)
	}
}

func TestResponseStatusThenXMLWritesStatusAndContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	res := &Response{writer: rec}

	type payload struct {
		XMLName xml.Name `xml:"payload"`
		Status  string   `xml:"status"`
	}

	if err := res.Status(http.StatusAccepted).XML(payload{Status: "accepted"}); err != nil {
		t.Fatalf("unexpected XML error: %v", err)
	}

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/xml" {
		t.Fatalf("expected content-type application/xml, got %q", got)
	}
}

func TestRequestBodyCachesBytesAcrossMultipleCalls(t *testing.T) {
	req := &Request{http: httptest.NewRequest(http.MethodPost, "/", strings.NewReader("golpher"))}

	first := string(req.Body().Bytes())
	second := string(req.Body().Bytes())

	if first != "golpher" || second != "golpher" {
		t.Fatalf("expected cached body twice, got first=%q second=%q", first, second)
	}
}

func TestRequestBodyJSONDecodesFromCachedBody(t *testing.T) {
	req := &Request{http: httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"golpher"}`))}

	var payload struct {
		Name string `json:"name"`
	}

	if err := req.Body().JSON(&payload); err != nil {
		t.Fatalf("unexpected JSON decode error: %v", err)
	}
	if payload.Name != "golpher" {
		t.Fatalf("expected decoded name golpher, got %q", payload.Name)
	}
}

func TestRequestBodyXMLDecodesFromCachedBody(t *testing.T) {
	req := &Request{http: httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`<payload><name>golpher</name></payload>`))}

	var payload struct {
		Name string `xml:"name"`
	}

	if err := req.Body().XML(&payload); err != nil {
		t.Fatalf("unexpected XML decode error: %v", err)
	}
	if payload.Name != "golpher" {
		t.Fatalf("expected decoded name golpher, got %q", payload.Name)
	}
}

func TestRequestContextExposesNativeContext(t *testing.T) {
	nativeCtx := context.WithValue(context.Background(), contextKey("golpher-test"), "ok")
	httpReq := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(nativeCtx)
	req := &Request{http: httpReq}

	if req.Context().Value(contextKey("golpher-test")) != "ok" {
		t.Fatal("expected request wrapper to expose native request context")
	}
}

func TestRequestContextCanBeCancelled(t *testing.T) {
	nativeCtx, cancel := context.WithCancel(context.Background())
	httpReq := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(nativeCtx)
	req := &Request{http: httpReq}

	cancel()

	select {
	case <-req.Context().Done():
		if !errors.Is(req.Context().Err(), context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", req.Context().Err())
		}
	default:
		t.Fatal("expected wrapped request context to be cancelled")
	}
}

func TestRequestBodyCachesReadError(t *testing.T) {
	expectedErr := errors.New("read failed")
	httpReq := httptest.NewRequest(http.MethodPost, "/", nil)
	httpReq.Body = failingReadCloser{err: expectedErr}
	req := &Request{http: httpReq}

	firstErr := req.Body().JSON(&struct{}{})
	secondErr := req.Body().XML(&struct{}{})

	if !errors.Is(firstErr, expectedErr) {
		t.Fatalf("expected first cached error %v, got %v", expectedErr, firstErr)
	}
	if !errors.Is(secondErr, expectedErr) {
		t.Fatalf("expected second cached error %v, got %v", expectedErr, secondErr)
	}
}

func TestTLSServerNegotiatesHTTP2WhenSupported(t *testing.T) {
	app := New()
	app.Router.GET("/proto", func(_ *Request, res *Response) error {
		return res.Status(http.StatusOK).JSON(map[string]string{"message": "ok"})
	})

	server := httptest.NewUnstartedServer(app)
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	resp, err := server.Client().Get(server.URL + "/proto")
	if err != nil {
		t.Fatalf("unexpected GET error: %v", err)
	}
	defer resp.Body.Close()

	if resp.ProtoMajor != 2 {
		t.Fatalf("expected HTTP/2, got %s", resp.Proto)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
}

type failingReadCloser struct {
	err error
}

func (f failingReadCloser) Read(_ []byte) (int, error) {
	return 0, f.err
}

func (f failingReadCloser) Close() error {
	return nil
}

var _ io.ReadCloser = failingReadCloser{}

type contextKey string
