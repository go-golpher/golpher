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
	app.GET("/hello", func(_ *Request, res *Response) error {
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

func TestAppPOSTRegistersRouteAndDispatchesHandler(t *testing.T) {
	app := New()
	app.POST("/items", func(_ *Request, res *Response) error {
		return res.Status(http.StatusCreated).String("created")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/items", nil)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "created" {
		t.Fatalf("expected body created, got %q", rec.Body.String())
	}
}

func TestMiddlewareChainExecutesInRegistrationOrder(t *testing.T) {
	app := New()
	var calls []string
	app.Use(func(next HandlerFunc) HandlerFunc {
		return func(req *Request, res *Response) error {
			calls = append(calls, "first-before")
			err := next(req, res)
			calls = append(calls, "first-after")
			return err
		}
	})
	app.Use(func(next HandlerFunc) HandlerFunc {
		return func(req *Request, res *Response) error {
			calls = append(calls, "second-before")
			err := next(req, res)
			calls = append(calls, "second-after")
			return err
		}
	})
	app.GET("/chain", func(_ *Request, res *Response) error {
		calls = append(calls, "handler")
		return res.Status(http.StatusOK).String("ok")
	})

	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/chain", nil))

	expected := []string{"first-before", "second-before", "handler", "second-after", "first-after"}
	if strings.Join(calls, ",") != strings.Join(expected, ",") {
		t.Fatalf("expected middleware order %v, got %v", expected, calls)
	}
}

func TestMiddlewareCanShortCircuitHandlerExecution(t *testing.T) {
	app := New()
	var handlerCalled bool
	app.Use(func(next HandlerFunc) HandlerFunc {
		return func(req *Request, res *Response) error {
			return req.NewError(http.StatusUnauthorized, "unauthorized")
		}
	})
	app.GET("/private", func(_ *Request, res *Response) error {
		handlerCalled = true
		return res.Status(http.StatusOK).String("secret")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/private", nil))

	if handlerCalled {
		t.Fatal("expected middleware to short-circuit handler execution")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRouterSupportsPathParams(t *testing.T) {
	app := New()
	app.GET("/users/:id", func(req *Request, res *Response) error {
		return res.Status(http.StatusOK).String(req.Param("id"))
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/42", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "42" {
		t.Fatalf("expected param 42, got %q", rec.Body.String())
	}
}

func TestAppWrapsStandardHTTPMiddleware(t *testing.T) {
	app := New()
	app.UseHTTP(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Stdlib-Middleware", "ok")
			next.ServeHTTP(w, r)
		})
	})
	app.GET("/stdlib", func(_ *Request, res *Response) error {
		return res.Status(http.StatusOK).String("ok")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stdlib", nil))

	if got := rec.Header().Get("X-Stdlib-Middleware"); got != "ok" {
		t.Fatalf("expected stdlib middleware header, got %q", got)
	}
}

func TestUseHTTPMiddlewareObservesGolpherErrorResponse(t *testing.T) {
	app := New()
	var observedStatus int
	app.UseHTTP(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)
			observedStatus = recorder.status
		})
	})
	app.GET("/error", func(req *Request, _ *Response) error {
		return req.NewError(http.StatusTeapot, "teapot")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/error", nil))

	if rec.Code != http.StatusTeapot {
		t.Fatalf("expected status %d, got %d", http.StatusTeapot, rec.Code)
	}
	if observedStatus != http.StatusTeapot {
		t.Fatalf("expected stdlib middleware to observe status %d, got %d", http.StatusTeapot, observedStatus)
	}
}

func TestAppMountsStandardHTTPHandler(t *testing.T) {
	app := New()
	app.Handle(http.MethodGet, "/mounted", FromHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("mounted"))
	})))

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mounted", nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if rec.Body.String() != "mounted" {
		t.Fatalf("expected mounted body, got %q", rec.Body.String())
	}
}

func TestGroupRegistersRoutesWithPrefixAndMiddleware(t *testing.T) {
	app := New()
	api := app.Group("/api", func(next HandlerFunc) HandlerFunc {
		return func(req *Request, res *Response) error {
			res.Header().Set("X-Group", "api")
			return next(req, res)
		}
	})
	api.GET("/users/:id", func(req *Request, res *Response) error {
		return res.Status(http.StatusOK).String(req.Param("id"))
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users/7", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if rec.Header().Get("X-Group") != "api" {
		t.Fatalf("expected group middleware header")
	}
	if strings.TrimSpace(rec.Body.String()) != "7" {
		t.Fatalf("expected param 7, got %q", rec.Body.String())
	}
}

func TestRecoverMiddlewareConvertsPanicToInternalServerError(t *testing.T) {
	app := New()
	app.Use(Recover())
	app.GET("/panic", func(_ *Request, _ *Response) error {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Fatalf("expected sanitized panic response, got %q", rec.Body.String())
	}
}

func TestBodyLimitRejectsPayloadTooLarge(t *testing.T) {
	app := New()
	app.Use(BodyLimit(4))
	app.POST("/payload", func(req *Request, res *Response) error {
		_ = req.Body().Bytes()
		return res.Status(http.StatusOK).String("ok")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/payload", strings.NewReader("too-large")))

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d", http.StatusRequestEntityTooLarge, rec.Code)
	}
}

func TestBodyLimitRejectsPayloadTooLargeWithoutContentLength(t *testing.T) {
	app := New()
	app.Use(BodyLimit(4))
	app.POST("/payload", func(_ *Request, res *Response) error {
		return res.Status(http.StatusOK).String("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/payload", io.NopCloser(strings.NewReader("too-large")))
	req.ContentLength = -1
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d", http.StatusRequestEntityTooLarge, rec.Code)
	}
}

func TestBodyLimitKeepsAllowedBodyReadable(t *testing.T) {
	app := New()
	app.Use(BodyLimit(16))
	app.POST("/payload", func(req *Request, res *Response) error {
		return res.Status(http.StatusOK).String(string(req.Body().Bytes()))
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/payload", strings.NewReader("golpher")))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "golpher" {
		t.Fatalf("expected body golpher, got %q", rec.Body.String())
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(status int) {
	rec.status = status
	rec.ResponseWriter.WriteHeader(status)
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
	app.Router.PUT("/resource", func(_ *Request, res *Response) error {
		return res.Status(http.StatusOK).JSON(map[string]string{"message": "ok"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/resource", nil)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET, PUT" {
		t.Fatalf("expected Allow header GET, PUT, got %q", got)
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

func TestResponseSendStoresBodySnapshot(t *testing.T) {
	rec := httptest.NewRecorder()
	res := &Response{writer: rec}

	if err := res.Status(http.StatusOK).Send([]byte("golpher")); err != nil {
		t.Fatalf("unexpected send error: %v", err)
	}

	if string(res.Body()) != "golpher" {
		t.Fatalf("expected body snapshot golpher, got %q", string(res.Body()))
	}
	if res.BodyString() != "golpher" {
		t.Fatalf("expected body string golpher, got %q", res.BodyString())
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
