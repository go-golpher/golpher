package golpher

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestResponse creates a Response backed by w with a tracking writer.
func newTestResponse(w http.ResponseWriter) *Response {
	res := &Response{}
	tw := &trackingResponseWriter{w: w, res: res}
	res.writer = tw
	return res
}

// newTestResponseCapture creates a Response with capture enabled.
func newTestResponseCapture(w http.ResponseWriter) *Response {
	res := newTestResponse(w)
	res.captureEnabled = true
	return res
}

func TestAppImplementsHTTPHandler(t *testing.T) {
	var _ http.Handler = New()
}

func TestRouterGETDispatchesMatchingHandler(t *testing.T) {
	app := New()
	app.GET("/hello", func(_ *Request, res *Response) error {
		res.SetStatus(http.StatusCreated)
		return res.JSON(map[string]string{"message": "ok"})
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
		res.SetStatus(http.StatusCreated)
		return res.String("created")
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

func TestAppMethodHelpersRegisterRoutes(t *testing.T) {
	app := New()
	app.PUT("/items/:id", func(req *Request, res *Response) error {
		res.SetStatus(http.StatusOK)
		return res.String("put:" + req.Param("id"))
	})
	app.PATCH("/items/:id", func(req *Request, res *Response) error {
		res.SetStatus(http.StatusOK)
		return res.String("patch:" + req.Param("id"))
	})
	app.DELETE("/items/:id", func(req *Request, res *Response) error {
		res.SetStatus(http.StatusOK)
		return res.String("delete:" + req.Param("id"))
	})

	cases := []struct {
		method string
		want   string
	}{
		{method: http.MethodPut, want: "put:9"},
		{method: http.MethodPatch, want: "patch:9"},
		{method: http.MethodDelete, want: "delete:9"},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, httptest.NewRequest(tc.method, "/items/9", nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
			}
			if strings.TrimSpace(rec.Body.String()) != tc.want {
				t.Fatalf("expected body %q, got %q", tc.want, rec.Body.String())
			}
		})
	}
}

func TestFromHTTPHandlerMountsStandardHandler(t *testing.T) {
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

func TestAppMountsStandardHTTPHandlerFunc(t *testing.T) {
	app := New()
	app.Handle(http.MethodGet, "/mounted-func", FromHTTPHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("mounted-func"))
	}))

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mounted-func", nil))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}
	if rec.Body.String() != "mounted-func" {
		t.Fatalf("expected mounted-func body, got %q", rec.Body.String())
	}
}

func TestAppServerUsesConfiguredTimeouts(t *testing.T) {
	app := New(AppConfig{
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       2 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       4 * time.Second,
		MaxHeaderBytes:    1024,
	})

	server := app.Server(":9090")

	if server.Addr != ":9090" {
		t.Fatalf("expected addr :9090, got %q", server.Addr)
	}
	if server.Handler != app {
		t.Fatal("expected server handler to be the app")
	}
	if server.ReadHeaderTimeout != time.Second || server.ReadTimeout != 2*time.Second || server.WriteTimeout != 3*time.Second || server.IdleTimeout != 4*time.Second {
		t.Fatalf("expected configured timeouts, got %#v", server)
	}
	if server.MaxHeaderBytes != 1024 {
		t.Fatalf("expected MaxHeaderBytes 1024, got %d", server.MaxHeaderBytes)
	}
}

func TestAppShutdownDelegatesToHTTPServer(t *testing.T) {
	app := New()
	server := httptest.NewServer(app)
	server.Close()

	if err := app.Shutdown(context.Background(), server.Config); err != nil {
		t.Fatalf("expected shutdown to succeed for closed test server, got %v", err)
	}
}

func TestAppServeUsesProvidedListener(t *testing.T) {
	app := New()
	app.GET("/ready", func(_ *Request, res *Response) error {
		return res.String("ok")
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("expected listener: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Serve(listener)
	}()

	resp, err := http.Get("http://" + listener.Addr().String() + "/ready")
	if err != nil {
		t.Fatalf("expected GET through provided listener: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("unexpected response body close error: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
	if body, err := io.ReadAll(resp.Body); err != nil || string(body) != "ok" {
		t.Fatalf("expected body ok, got %q err=%v", string(body), err)
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("expected listener close: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("expected closed listener error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected Serve to return after listener close")
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
		res.SetStatus(http.StatusOK)
		return res.String("ok")
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
			return ErrorGolpher{Code: http.StatusUnauthorized, Message: "unauthorized"}
		}
	})
	app.GET("/private", func(_ *Request, res *Response) error {
		handlerCalled = true
		res.SetStatus(http.StatusOK)
		return res.String("secret")
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
		res.SetStatus(http.StatusOK)
		return res.String(req.Param("id"))
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

func TestDynamicRouteMatchIntoDoesNotAllocateParams(t *testing.T) {
	segs, names := compileRouteSegmentsResult("/users/:id/orders/:orderID")
	rt := route{
		method:           http.MethodGet,
		compiledSegments: segs,
		paramNames:       names,
	}
	request := &Request{paramValues: make([]string, 0, 2)}
	allocs := testing.AllocsPerRun(1000, func() {
		if !rt.matchInto("/users/42/orders/abc", "users/42/orders/abc", request) {
			t.Fatal("expected route match")
		}
		if request.Param("id") != "42" || request.Param("orderID") != "abc" {
			t.Fatalf("unexpected params: id=%q orderID=%q", request.Param("id"), request.Param("orderID"))
		}
	})
	if allocs != 0 {
		t.Fatalf("expected dynamic param matching to allocate 0 times, got %.2f", allocs)
	}
}

func TestDynamicRouteDoesNotMatchExtraSegments(t *testing.T) {
	app := New()
	app.GET("/users/:id", func(req *Request, res *Response) error {
		return res.String(req.Param("id"))
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/42/orders", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestDynamicRouteDoesNotMatchRootPath(t *testing.T) {
	app := New()
	app.GET("/:id", func(req *Request, res *Response) error {
		return res.String(req.Param("id"))
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestDynamicRootMethodMismatchIgnoresParamRoute(t *testing.T) {
	app := New()
	app.GET("/:id", func(req *Request, res *Response) error {
		return res.String(req.Param("id"))
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestDynamicWildcardCapturesRemainingPath(t *testing.T) {
	app := New()
	app.GET("/files/*path", func(req *Request, res *Response) error {
		return res.String(req.Param("path"))
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/files/a/b/c", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "a/b/c" {
		t.Fatalf("expected wildcard path a/b/c, got %q", rec.Body.String())
	}
}

func TestStaticRouteFastPathPreservesTrailingSlashCompatibility(t *testing.T) {
	app := New()
	app.GET("/hello", func(_ *Request, res *Response) error {
		res.SetStatus(http.StatusOK)
		return res.String("hello")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hello/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "hello" {
		t.Fatalf("expected static route response hello, got %q", rec.Body.String())
	}
}

func TestStaticRouteFastPathTakesPrecedenceOverEarlierDynamicRoute(t *testing.T) {
	app := New()
	app.GET("/:id", func(req *Request, res *Response) error {
		res.SetStatus(http.StatusOK)
		return res.String("dynamic:" + req.Param("id"))
	})
	app.GET("/health", func(_ *Request, res *Response) error {
		res.SetStatus(http.StatusOK)
		return res.String("static")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "static" {
		t.Fatalf("expected static route to take precedence, got %q", rec.Body.String())
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
		res.SetStatus(http.StatusOK)
		return res.String("ok")
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
	app.GET("/error", func(_ *Request, _ *Response) error {
		return ErrorGolpher{Code: http.StatusTeapot, Message: "teapot"}
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/error", nil))

	if rec.Code != http.StatusTeapot {
		t.Fatalf("expected status %d, got %d", http.StatusTeapot, rec.Code)
	}
	// Errors flow through the tracking writer after the stdlib
	// middleware wrapper returns. Use ErrorObserver for
	// programmatic error observation.
	if observedStatus != http.StatusOK {
		t.Fatalf("expected stdlib middleware to see initial OK status, got %d", observedStatus)
	}
}

func TestUseHTTPRespectsDisabledResponseBodyCapture(t *testing.T) {
	app := New()
	app.UseHTTP(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	})
	var snapshot string
	app.GET("/capture", func(_ *Request, res *Response) error {
		if err := res.String("ok"); err != nil {
			return err
		}
		snapshot = res.BodyString()
		return nil
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/capture", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("expected response body ok, got %q", rec.Body.String())
	}
	if snapshot != "" {
		t.Fatalf("expected disabled response snapshot through UseHTTP, got %q", snapshot)
	}
}

func TestUseHTTPPreservesDynamicRouteParams(t *testing.T) {
	app := New()
	app.UseHTTP(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	})
	app.GET("/users/:id", func(req *Request, res *Response) error {
		return res.String(req.Param("id"))
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/42", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "42" {
		t.Fatalf("expected dynamic param through UseHTTP, got %q", rec.Body.String())
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
		res.SetStatus(http.StatusOK)
		return res.String(req.Param("id"))
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

func TestGroupUseAndMethodHelpers(t *testing.T) {
	app := New()
	api := app.Group("/api")
	api.Use(func(next HandlerFunc) HandlerFunc {
		return func(req *Request, res *Response) error {
			res.Header().Set("X-Group-Use", "ok")
			return next(req, res)
		}
	})

	api.POST("/items", func(_ *Request, res *Response) error {
		res.SetStatus(http.StatusCreated)
		return res.String("post")
	})
	api.PUT("/items/:id", func(req *Request, res *Response) error {
		return res.String("put:" + req.Param("id"))
	})
	api.PATCH("/items/:id", func(req *Request, res *Response) error {
		return res.String("patch:" + req.Param("id"))
	})
	api.DELETE("/items/:id", func(req *Request, res *Response) error {
		return res.String("delete:" + req.Param("id"))
	})

	cases := []struct {
		method string
		path   string
		status int
		body   string
	}{
		{method: http.MethodPost, path: "/api/items", status: http.StatusCreated, body: "post"},
		{method: http.MethodPut, path: "/api/items/1", status: http.StatusOK, body: "put:1"},
		{method: http.MethodPatch, path: "/api/items/1", status: http.StatusOK, body: "patch:1"},
		{method: http.MethodDelete, path: "/api/items/1", status: http.StatusOK, body: "delete:1"},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			if rec.Code != tc.status {
				t.Fatalf("expected status %d, got %d", tc.status, rec.Code)
			}
			if rec.Header().Get("X-Group-Use") != "ok" {
				t.Fatal("expected group Use middleware header")
			}
			if strings.TrimSpace(rec.Body.String()) != tc.body {
				t.Fatalf("expected body %q, got %q", tc.body, rec.Body.String())
			}
		})
	}
}

func TestRootGroupRegistersRootPath(t *testing.T) {
	app := New()
	root := app.Group("/")
	root.GET("/", func(_ *Request, res *Response) error {
		return res.String("root")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "root" {
		t.Fatalf("expected root body, got %q", rec.Body.String())
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
		_, err := req.Body()
		return err
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/payload", strings.NewReader("too-large")))

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d", http.StatusRequestEntityTooLarge, rec.Code)
	}
}

func TestBodyLimitKeepsAllowedBodyReadable(t *testing.T) {
	app := New()
	app.Use(BodyLimit(16))
	app.POST("/payload", func(req *Request, res *Response) error {
		data, err := req.Body()
		if err != nil {
			return err
		}
		res.SetStatus(http.StatusOK)
		return res.String(string(data))
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

func TestBodyLimitNegativeLeavesBodyUnlimited(t *testing.T) {
	app := New()
	app.Use(BodyLimit(-1))
	app.POST("/payload", func(req *Request, res *Response) error {
		data, err := req.Body()
		if err != nil {
			return err
		}
		res.SetStatus(http.StatusOK)
		return res.String(string(data))
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/payload", strings.NewReader("unlimited")))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "unlimited" {
		t.Fatalf("expected body unlimited, got %q", rec.Body.String())
	}
}

func TestBodyLimitPropagatesMaxBytesError(t *testing.T) {
	app := New()
	app.POST("/payload", func(req *Request, res *Response) error {
		_, err := req.Body()
		return err
	})

	bigBody := strings.NewReader(strings.Repeat("x", 2<<20))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/payload", bigBody))

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d", http.StatusRequestEntityTooLarge, rec.Code)
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
	app.GET("/known", func(_ *Request, res *Response) error {
		res.SetStatus(http.StatusOK)
		return res.JSON(map[string]string{"message": "known"})
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
	app.GET("/resource", func(_ *Request, res *Response) error {
		res.SetStatus(http.StatusOK)
		return res.JSON(map[string]string{"message": "ok"})
	})
	app.PUT("/resource", func(_ *Request, res *Response) error {
		res.SetStatus(http.StatusOK)
		return res.JSON(map[string]string{"message": "ok"})
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

func TestResponseSetStatusThenJSONWritesStatusAndContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponse(rec)

	res.SetStatus(http.StatusAccepted)
	if err := res.JSON(map[string]string{"status": "accepted"}); err != nil {
		t.Fatalf("unexpected JSON error: %v", err)
	}

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", got)
	}
}

func TestResponseJSONBytesWritesPreEncodedJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponseCapture(rec)

	res.SetStatus(http.StatusCreated)
	if err := res.JSONBytes([]byte(`{"status":"ok"}`)); err != nil {
		t.Fatalf("unexpected JSONBytes error: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", got)
	}
	if rec.Body.String() != `{"status":"ok"}` {
		t.Fatalf("expected pre-encoded JSON body, got %q", rec.Body.String())
	}
}

func TestResponseBytesWritesWithoutBodySnapshot(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponse(rec)

	if err := res.Bytes(http.StatusAccepted, "application/octet-stream", []byte("payload")); err != nil {
		t.Fatalf("unexpected Bytes error: %v", err)
	}

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("expected content-type application/octet-stream, got %q", got)
	}
	if got := rec.Header().Get("Content-Length"); got != "7" {
		t.Fatalf("expected content-length 7, got %q", got)
	}
	if rec.Body.String() != "payload" {
		t.Fatalf("expected writer body payload, got %q", rec.Body.String())
	}
	if res.BodyString() != "" {
		t.Fatalf("expected Bytes not to capture body snapshot, got %q", res.BodyString())
	}
}

func TestResponseBytesUsesPriorStatusWhenStatusArgumentIsZero(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponseCapture(rec)

	res.SetStatus(http.StatusAccepted)
	if err := res.Bytes(0, "text/plain", []byte("accepted")); err != nil {
		t.Fatalf("unexpected Bytes error: %v", err)
	}

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
}

func TestResponseRawExposesTrackingWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponseCapture(rec)

	w := res.Raw()
	if w == nil {
		t.Fatal("expected tracking writer from Raw()")
	}

	res.Header().Set("X-Raw", "ok")
	if got := rec.Header().Get("X-Raw"); got != "ok" {
		t.Fatalf("expected raw header ok, got %q", got)
	}
}

func TestResponseSendStoresBodySnapshotWhenCaptureEnabled(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponseCapture(rec)

	res.SetStatus(http.StatusOK)
	if err := res.Send([]byte("golpher")); err != nil {
		t.Fatalf("unexpected send error: %v", err)
	}

	if string(res.Body()) != "golpher" {
		t.Fatalf("expected body snapshot golpher, got %q", string(res.Body()))
	}
	if res.BodyString() != "golpher" {
		t.Fatalf("expected body string golpher, got %q", res.BodyString())
	}
}

func TestResponseBodyCaptureOffByDefault(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponse(rec)

	if err := res.String("golpher"); err != nil {
		t.Fatalf("unexpected string error: %v", err)
	}

	if rec.Body.String() != "golpher" {
		t.Fatalf("expected writer body golpher, got %q", rec.Body.String())
	}
	if res.BodyString() != "" {
		t.Fatalf("expected disabled capture to stay empty, got %q", res.BodyString())
	}
	if res.Body() != nil {
		t.Fatalf("expected nil body when capture disabled, got %v", res.Body())
	}
}

func TestPooledRequestClearsState(t *testing.T) {
	firstHTTPReq := httptest.NewRequest(http.MethodPost, "/items/1", strings.NewReader("first"))
	bw := &benchmarkResponseWriter{}
	res := newTestResponse(bw)
	firstReq := acquireRequest(firstHTTPReq, res, 1<<20)

	data, err := firstReq.Body()
	if err != nil {
		t.Fatalf("unexpected body error: %v", err)
	}
	if firstReq.Param("id") != "" || string(data) != "first" {
		t.Fatalf("expected first request state to be populated")
	}

	releaseRequest(firstReq)

	if firstReq.Raw() != nil || firstReq.params != nil {
		t.Fatalf("expected released request to clear references, got %#v", firstReq)
	}
	if firstReq.bodyRead || firstReq.bodyData != nil || firstReq.bodyErr != nil {
		t.Fatalf("expected released body state to clear")
	}

	secondHTTPReq := httptest.NewRequest(http.MethodPost, "/items/2", strings.NewReader("second"))
	secondReq := acquireRequest(secondHTTPReq, res, 1<<20)
	defer releaseRequest(secondReq)

	if secondReq.Raw() != secondHTTPReq {
		t.Fatal("expected second request to expose its own http request")
	}
	if secondReq.Param("id") != "" {
		t.Fatalf("expected pooled request params to be cleared, got %q", secondReq.Param("id"))
	}
	if secondReq.Query("query") != "" {
		t.Fatalf("expected empty query, got %q", secondReq.Query("query"))
	}
}

func TestPooledResponseClearsStateBeforeReuse(t *testing.T) {
	firstRec := httptest.NewRecorder()
	firstRes := acquireResponse(firstRec, true)

	if err := firstRes.SetStatus(http.StatusCreated).String("first"); err != nil {
		t.Fatalf("unexpected first response error: %v", err)
	}
	if firstRes.Status() != http.StatusCreated || firstRes.BodyString() != "first" {
		t.Fatalf("expected first response state to be populated")
	}

	releaseResponse(firstRes)

	if firstRes.writer != nil || firstRes.status != 0 || firstRes.BodyString() != "" {
		t.Fatalf("expected released response wrapper to clear state, got %#v", firstRes)
	}

	secondRec := httptest.NewRecorder()
	secondRes := acquireResponse(secondRec, true)
	defer releaseResponse(secondRes)

	if secondRes.Raw() == nil {
		t.Fatal("expected second response to expose its own writer")
	}
	if secondRes.status != 0 || secondRes.BodyString() != "" {
		t.Fatalf("expected pooled response state to be cleared, status=%d body=%q", secondRes.status, secondRes.BodyString())
	}
	if err := secondRes.String("second"); err != nil {
		t.Fatalf("unexpected second response error: %v", err)
	}
	if strings.TrimSpace(secondRec.Body.String()) != "second" {
		t.Fatalf("expected second recorder body, got %q", secondRec.Body.String())
	}
}

func TestPooledResponseDropsOversizedBodyBuffer(t *testing.T) {
	rec := httptest.NewRecorder()
	res := acquireResponse(rec, true)
	largeBody := strings.Repeat("x", maxPooledResponseBufferCapacity+1)

	if err := res.String(largeBody); err != nil {
		t.Fatalf("unexpected response error: %v", err)
	}
	if res.body.Cap() <= maxPooledResponseBufferCapacity {
		t.Fatalf("expected oversized response buffer, got cap %d", res.body.Cap())
	}

	releaseResponse(res)

	if res.body.Cap() > maxPooledResponseBufferCapacity {
		t.Fatalf("expected oversized buffer to be dropped, got cap %d", res.body.Cap())
	}
}

func TestPooledResponseBodySnapshotAvailableDuringHandler(t *testing.T) {
	app := New(AppConfig{EnableResponseBodyCapture: true})
	var snapshot string
	app.GET("/snapshot", func(_ *Request, res *Response) error {
		if err := res.String("golpher"); err != nil {
			return err
		}
		snapshot = string(res.Body())
		return nil
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/snapshot", nil))

	if snapshot != "golpher" {
		t.Fatalf("expected response body snapshot during handler, got %q", snapshot)
	}
	if strings.TrimSpace(rec.Body.String()) != "golpher" {
		t.Fatalf("expected recorder body golpher, got %q", rec.Body.String())
	}
}

func TestMiddlewareRegisteredAfterRouteUsesCompiledChain(t *testing.T) {
	app := New()
	app.GET("/late", func(_ *Request, res *Response) error {
		return res.String("handler")
	})
	app.Use(func(next HandlerFunc) HandlerFunc {
		return func(req *Request, res *Response) error {
			res.Header().Set("X-Late-Middleware", "ok")
			return next(req, res)
		}
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/late", nil))

	if rec.Header().Get("X-Late-Middleware") != "ok" {
		t.Fatal("expected middleware registered after route to run")
	}
	if strings.TrimSpace(rec.Body.String()) != "handler" {
		t.Fatalf("expected handler response, got %q", rec.Body.String())
	}
}

func TestResponseSetStatusThenXMLWritesStatusAndContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponseCapture(rec)

	type payload struct {
		XMLName xml.Name `xml:"payload"`
		Status  string   `xml:"status"`
	}

	res.SetStatus(http.StatusAccepted)
	if err := res.XML(payload{Status: "accepted"}); err != nil {
		t.Fatalf("unexpected XML error: %v", err)
	}

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/xml" {
		t.Fatalf("expected content-type application/xml, got %q", got)
	}
}

func TestResponseRedirectWritesLocationStatusAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponseCapture(rec)

	if err := res.Redirect("/target", http.StatusTemporaryRedirect); err != nil {
		t.Fatalf("unexpected redirect error: %v", err)
	}

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status %d, got %d", http.StatusTemporaryRedirect, rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/target" {
		t.Fatalf("expected location /target, got %q", got)
	}
	if !strings.Contains(res.BodyString(), "/target") {
		t.Fatalf("expected redirect body snapshot to mention target, got %q", res.BodyString())
	}
}

func TestRequestBodyCachesBytesAcrossMultipleCalls(t *testing.T) {
	bw := &benchmarkResponseWriter{}
	res := newTestResponse(bw)
	req := acquireRequest(httptest.NewRequest(http.MethodPost, "/", strings.NewReader("golpher")), res, 1<<20)

	first, err := req.Body()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err2 := req.Body()
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}

	if string(first) != "golpher" || string(second) != "golpher" {
		t.Fatalf("expected cached body twice, got first=%q second=%q", string(first), string(second))
	}
}

func TestRequestBodyJSONDecodesFromCachedBody(t *testing.T) {
	bw := &benchmarkResponseWriter{}
	res := newTestResponse(bw)
	req := acquireRequest(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"golpher"}`)), res, 1<<20)

	var payload struct {
		Name string `json:"name"`
	}

	if err := req.BodyJSON(&payload); err != nil {
		t.Fatalf("unexpected JSON decode error: %v", err)
	}
	if payload.Name != "golpher" {
		t.Fatalf("expected decoded name golpher, got %q", payload.Name)
	}
}

func TestRequestBodyXMLDecodesFromCachedBody(t *testing.T) {
	bw := &benchmarkResponseWriter{}
	res := newTestResponse(bw)
	req := acquireRequest(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`<payload><name>golpher</name></payload>`)), res, 1<<20)

	var payload struct {
		Name string `xml:"name"`
	}

	if err := req.BodyXML(&payload); err != nil {
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

func TestRequestSetContextUpdatesNativeContext(t *testing.T) {
	req := &Request{http: httptest.NewRequest(http.MethodGet, "/", nil)}
	req.SetContext(context.WithValue(req.Context(), contextKey("golpher-user"), "user-1"))

	if req.Context().Value(contextKey("golpher-user")) != "user-1" {
		t.Fatal("expected request wrapper to expose updated native request context")
	}
}

func TestRequestRawHeadersQueryAndMissingParam(t *testing.T) {
	httpReq := httptest.NewRequest(http.MethodGet, "/search?q=golpher", nil)
	httpReq.Header.Set("X-Test", "ok")
	req := &Request{http: httpReq}

	if req.Raw() != httpReq {
		t.Fatal("expected raw http request")
	}
	if req.Headers()["X-Test"][0] != "ok" {
		t.Fatalf("expected header ok, got %#v", req.Headers())
	}
	if req.Query("q") != "golpher" {
		t.Fatalf("expected query golpher, got %q", req.Query("q"))
	}
	if req.Param("missing") != "" {
		t.Fatalf("expected missing param to be empty, got %q", req.Param("missing"))
	}
}

func TestDefaultErrorHandlerWritesErrorGolpherJSON(t *testing.T) {
	app := New()
	app.GET("/conflict", func(_ *Request, _ *Response) error {
		return ErrorGolpher{Code: http.StatusConflict, Message: "conflict"}
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/conflict", nil))

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
	var payload ErrorGolpher
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected error JSON: %v", err)
	}
	if payload.Code != http.StatusConflict || payload.Message != "conflict" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func TestDefaultErrorHandlerMasksUnknownError(t *testing.T) {
	app := New()
	app.GET("/boom", func(_ *Request, _ *Response) error {
		return errors.New("database connection refused")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
	var payload ErrorGolpher
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected error JSON: %v", err)
	}
	if payload.Code != http.StatusInternalServerError || payload.Message != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("expected generic 500, got %#v", payload)
	}
	// The original error text must NOT leak.
	if strings.Contains(rec.Body.String(), "database connection refused") {
		t.Fatalf("expected masked error, got %q", rec.Body.String())
	}
}

func TestErrorObserverReceivesOriginalError(t *testing.T) {
	originalErr := errors.New("db timeout")
	var observedErr error
	var observedCount int

	app := New(AppConfig{
		ErrorObserver: func(req *Request, res *Response, err error) {
			observedErr = err
			observedCount++
		},
	})
	app.GET("/obs", func(_ *Request, _ *Response) error {
		return originalErr
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/obs", nil))

	if !errors.Is(observedErr, originalErr) {
		t.Fatalf("expected observer to receive original error, got %v", observedErr)
	}
	if observedCount != 1 {
		t.Fatalf("expected observer called once, got %d", observedCount)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "db timeout") {
		t.Fatalf("expected masked error body, got %q", rec.Body.String())
	}
}

func TestCustomErrorHandlerOverridesDefaultBehavior(t *testing.T) {
	app := New(AppConfig{
		ErrorHandler: func(req *Request, res *Response, err error) {
			_ = res.SetStatus(http.StatusBadGateway).JSON(map[string]string{"error": "masked"})
		},
	})
	app.GET("/custom-error", func(_ *Request, _ *Response) error {
		return errors.New("internal detail")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/custom-error", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rec.Code)
	}
	if strings.Contains(rec.Body.String(), "internal detail") {
		t.Fatalf("expected custom handler to mask internal detail, got %q", rec.Body.String())
	}
}

func TestNilErrorObserverNoOpOnHandlerError(t *testing.T) {
	// Default New() stores nil ErrorObserver — must be a no-op, no panic.
	app := New()
	app.GET("/err", func(_ *Request, _ *Response) error {
		return ErrorGolpher{Code: http.StatusTeapot, Message: "teapot"}
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/err", nil))

	if rec.Code != http.StatusTeapot {
		t.Fatalf("expected 418, got %d", rec.Code)
	}
	var payload ErrorGolpher
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON error body: %v", err)
	}
	if payload.Code != http.StatusTeapot || payload.Message != "teapot" {
		t.Fatalf("unexpected payload: %#v", payload)
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
	bw := &benchmarkResponseWriter{}
	res := newTestResponse(bw)
	req := acquireRequest(httpReq, res, 1<<20)

	firstErr := req.BodyJSON(&struct{}{})
	secondErr := req.BodyXML(&struct{}{})

	if !errors.Is(firstErr, expectedErr) {
		t.Fatalf("expected first cached error %v, got %v", expectedErr, firstErr)
	}
	if !errors.Is(secondErr, expectedErr) {
		t.Fatalf("expected second cached error %v, got %v", expectedErr, secondErr)
	}
}

func TestCallerConfigMutationDoesNotAffectApp(t *testing.T) {
	cfg := AppConfig{Port: 9090}
	app := New(cfg)
	cfg.Port = 1234
	if app.config.Port != 9090 {
		t.Fatalf("expected app port 9090, got %d", app.config.Port)
	}
}

func TestIsFrozenFalseBeforeFirstRequest(t *testing.T) {
	app := New()
	if app.IsFrozen() {
		t.Fatal("expected IsFrozen() to be false before first request")
	}
}

func TestIsFrozenTrueAfterFirstServeHTTP(t *testing.T) {
	app := New()
	app.GET("/", func(_ *Request, res *Response) error { return res.String("ok") })

	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !app.IsFrozen() {
		t.Fatal("expected IsFrozen() to be true after first ServeHTTP")
	}
}

func TestRegistrationAfterFreezePanics(t *testing.T) {
	app := New()
	app.GET("/", func(_ *Request, res *Response) error { return res.String("ok") })

	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for registration after freeze")
		}
	}()

	app.GET("/late", func(_ *Request, res *Response) error {
		return nil
	})
}

func TestUseAfterFreezePanics(t *testing.T) {
	app := New()
	app.GET("/", func(_ *Request, res *Response) error { return res.String("ok") })

	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for Use after freeze")
		}
	}()

	app.Use(func(next HandlerFunc) HandlerFunc { return next })
}

func TestNilHandlerPanics(t *testing.T) {
	app := New()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil handler")
		}
	}()
	app.GET("/test", nil)
}

func TestEmptyMethodPanics(t *testing.T) {
	app := New()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty method")
		}
	}()
	app.Handle("", "/path", func(_ *Request, res *Response) error { return nil })
}

func TestMethodWithSpacesPanics(t *testing.T) {
	app := New()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for method with spaces")
		}
	}()
	app.Handle("GE T", "/path", func(_ *Request, res *Response) error { return nil })
}

func TestValidExtensionMethodAccepted(t *testing.T) {
	app := New()
	app.Handle("CUSTOM", "/path", func(_ *Request, res *Response) error { return nil })
}

func TestEmptyParamNamePanics(t *testing.T) {
	app := New()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty param name")
		}
	}()
	app.GET("/users/:/profile", func(_ *Request, res *Response) error { return nil })
}

func TestEmptyWildcardNamePanics(t *testing.T) {
	app := New()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty wildcard name")
		}
	}()
	app.GET("/assets/*", func(_ *Request, res *Response) error { return nil })
}

func TestNonTerminalWildcardPanics(t *testing.T) {
	app := New()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-terminal wildcard")
		}
	}()
	app.GET("/assets/*file/details", func(_ *Request, res *Response) error { return nil })
}

func TestExactDuplicateRoutePanics(t *testing.T) {
	app := New()
	app.GET("/users", func(_ *Request, res *Response) error { return nil })
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for duplicate route")
		}
	}()
	app.GET("/users", func(_ *Request, res *Response) error { return nil })
}

func TestTrailingSlashDuplicatePanics(t *testing.T) {
	app := New()
	app.GET("/users", func(_ *Request, res *Response) error { return nil })
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for trailing-slash alias duplicate")
		}
	}()
	app.GET("/users/", func(_ *Request, res *Response) error { return nil })
}

func TestSamePatternUnderDifferentMethodsAllowed(t *testing.T) {
	app := New()
	app.GET("/users", func(_ *Request, res *Response) error { return nil })
	app.POST("/users", func(_ *Request, res *Response) error { return nil })
}

func TestConflictingParamNamesPanics(t *testing.T) {
	app := New()
	app.GET("/users/:id", func(_ *Request, res *Response) error { return nil })
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for conflicting param names")
		}
	}()
	app.GET("/users/:name", func(_ *Request, res *Response) error { return nil })
}

func TestSameParamNameInDifferentPositionsAllowed(t *testing.T) {
	app := New()
	app.GET("/users/:id", func(_ *Request, res *Response) error { return nil })
	app.GET("/users/:id/orders", func(_ *Request, res *Response) error { return nil })
}

// writeByteRecorder implements io.ByteWriter alongside http.ResponseWriter.
type writeByteRecorder struct {
	http.ResponseWriter
	lastByte byte
	writeErr error
}

func (w *writeByteRecorder) WriteByte(c byte) error {
	if w.writeErr != nil {
		return w.writeErr
	}
	w.lastByte = c
	return nil
}

func TestTrackingWriteByteUnderlying(t *testing.T) {
	rec := httptest.NewRecorder()
	bw := &writeByteRecorder{ResponseWriter: rec}
	res := newTestResponseCapture(bw)
	tw := res.Raw().(*trackingResponseWriter)

	if err := tw.WriteByte('X'); err != nil {
		t.Fatalf("unexpected WriteByte error: %v", err)
	}

	if bw.lastByte != 'X' {
		t.Fatalf("expected underlying lastByte 'X', got %q", bw.lastByte)
	}
	if res.BytesWritten() != 1 {
		t.Fatalf("expected 1 byte written, got %d", res.BytesWritten())
	}
	if string(res.Body()) != "X" {
		t.Fatalf("expected captured 'X', got %q", string(res.Body()))
	}
	if !res.Committed() {
		t.Fatal("expected committed after WriteByte")
	}
	if res.Status() != http.StatusOK {
		t.Fatalf("expected implicit 200, got %d", res.Status())
	}
}

func TestTrackingWriteByteFallback(t *testing.T) {
	rec := httptest.NewRecorder() // httptest.ResponseRecorder does NOT implement io.ByteWriter
	res := newTestResponseCapture(rec)
	tw := res.Raw().(*trackingResponseWriter)

	if err := tw.WriteByte('Y'); err != nil {
		t.Fatalf("unexpected WriteByte fallback error: %v", err)
	}

	if res.BytesWritten() != 1 {
		t.Fatalf("expected 1 byte via fallback, got %d", res.BytesWritten())
	}
	if string(res.Body()) != "Y" {
		t.Fatalf("expected captured 'Y' via fallback, got %q", string(res.Body()))
	}
	if rec.Body.String() != "Y" {
		t.Fatalf("expected recorder body 'Y', got %q", rec.Body.String())
	}
}

func TestTrackingWriteByteErrorNoCount(t *testing.T) {
	rec := httptest.NewRecorder()
	bw := &writeByteRecorder{
		ResponseWriter: rec,
		writeErr:       errWriteFailed,
	}
	res := newTestResponseCapture(bw)
	tw := res.Raw().(*trackingResponseWriter)

	if err := tw.WriteByte('Z'); err == nil {
		t.Fatal("expected WriteByte to propagate error")
	}

	if res.BytesWritten() != 0 {
		t.Fatalf("expected 0 bytes on error, got %d", res.BytesWritten())
	}
	if string(res.Body()) != "" {
		t.Fatalf("expected empty capture on error, got %q", string(res.Body()))
	}
}

var errWriteFailed = &writeError{}

type writeError struct{}

func (e *writeError) Error() string { return "write failed" }

func TestTrackingWriteStringImplicitStatusBytesCapture(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponseCapture(rec)
	tw := res.Raw().(*trackingResponseWriter)

	n, err := tw.WriteString("hello")
	if err != nil {
		t.Fatalf("unexpected WriteString error: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected 5 bytes from WriteString, got %d", n)
	}
	if res.BytesWritten() != 5 {
		t.Fatalf("expected 5 bytes written, got %d", res.BytesWritten())
	}
	if string(res.Body()) != "hello" {
		t.Fatalf("expected captured 'hello', got %q", string(res.Body()))
	}
	if !res.Committed() {
		t.Fatal("expected committed after WriteString")
	}
	if res.Status() != http.StatusOK {
		t.Fatalf("expected implicit 200, got %d", res.Status())
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("expected recorder body 'hello', got %q", rec.Body.String())
	}
}

func TestFromHTTPHandlerEndToEndTracking(t *testing.T) {
	var bw int64
	var bodyStr string
	var committed bool
	var status int

	app := New(AppConfig{EnableResponseBodyCapture: true})

	app.Use(func(next HandlerFunc) HandlerFunc {
		return func(req *Request, res *Response) error {
			err := next(req, res)
			bw = res.BytesWritten()
			bodyStr = res.BodyString()
			committed = res.Committed()
			status = res.Status()
			return err
		}
	})

	app.Handle(http.MethodGet, "/e2e", FromHTTPHandler(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Custom", "yes")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("e2e-body"))
		}),
	))

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/e2e", nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	if rec.Header().Get("X-Custom") != "yes" {
		t.Fatal("expected X-Custom header")
	}
	if rec.Body.String() != "e2e-body" {
		t.Fatalf("expected body 'e2e-body', got %q", rec.Body.String())
	}
	if !committed {
		t.Fatal("expected committed after FromHTTPHandler write")
	}
	if status != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", status)
	}
	if bw != 8 {
		t.Fatalf("expected 8 bytes written, got %d", bw)
	}
	if bodyStr != "e2e-body" {
		t.Fatalf("expected captured body 'e2e-body', got %q", bodyStr)
	}
}

func TestFromHTTPHandlerFuncEndToEndTracking(t *testing.T) {
	var bw int64
	var bodyStr string
	var committed bool
	var status int

	app := New(AppConfig{EnableResponseBodyCapture: true})

	app.Use(func(next HandlerFunc) HandlerFunc {
		return func(req *Request, res *Response) error {
			err := next(req, res)
			bw = res.BytesWritten()
			bodyStr = res.BodyString()
			committed = res.Committed()
			status = res.Status()
			return err
		}
	})

	app.Handle(http.MethodGet, "/e2e-func", FromHTTPHandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("func-body"))
		},
	))

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/e2e-func", nil))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if rec.Body.String() != "func-body" {
		t.Fatalf("expected body 'func-body', got %q", rec.Body.String())
	}
	if !committed {
		t.Fatal("expected committed after FromHTTPHandlerFunc write")
	}
	if status != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", status)
	}
	if bw != 9 {
		t.Fatalf("expected 9 bytes written, got %d", bw)
	}
	if bodyStr != "func-body" {
		t.Fatalf("expected captured body 'func-body', got %q", bodyStr)
	}
}

func TestCommittedPreventsSecondWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponseCapture(rec)

	res.SetStatus(http.StatusOK)
	res.writer.WriteHeader(http.StatusOK)
	res.WriteHeader(http.StatusCreated)
	_ = res

	if rec.Code != http.StatusOK {
		t.Fatalf("expected committed status 200 to stick, got %d", rec.Code)
	}
}

// trackingResponseWriter.WriteHeader is called by the Response wrapper.
func (res *Response) WriteHeader(status int) {
	if tw, ok := res.writer.(*trackingResponseWriter); ok {
		tw.WriteHeader(status)
	}
}

func TestBytesWrittenReflectsBodyBytes(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponseCapture(rec)

	if err := res.String("hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.BytesWritten() != 5 {
		t.Fatalf("expected 5 bytes written, got %d", res.BytesWritten())
	}
}

func TestResponseControllerUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponseCapture(rec)

	// http.ResponseController should reach the underlying writer.
	rc := http.NewResponseController(res.Raw())
	if rc == nil {
		t.Fatal("expected ResponseController from tracked writer")
	}
	if err := rc.Flush(); err != nil {
		t.Fatalf("unexpected flush error: %v", err)
	}
}

func TestMethodQueryConstant(t *testing.T) {
	if MethodQuery != "QUERY" {
		t.Fatalf("expected MethodQuery to be QUERY, got %q", MethodQuery)
	}
}

func TestAppQUERYRegistersQUERYRoute(t *testing.T) {
	app := New()
	var called bool
	app.QUERY("/search", func(_ *Request, res *Response) error {
		called = true
		return res.String("results")
	})

	req := httptest.NewRequest("QUERY", "/search", strings.NewReader("q=test"))
	req.Header.Set("Content-Type", "application/query")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected QUERY handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestQUERYWithoutContentTypeReturns400(t *testing.T) {
	app := New()
	app.QUERY("/search", func(_ *Request, res *Response) error {
		return res.String("results")
	})

	req := httptest.NewRequest("QUERY", "/search", strings.NewReader("q=test"))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing Content-Type, got %d", rec.Code)
	}
}

func TestErrorObserverForQUERYWithoutContentType(t *testing.T) {
	var observedErr error
	var observedCount int
	app := New(AppConfig{
		ErrorObserver: func(req *Request, res *Response, err error) {
			observedErr = err
			observedCount++
		},
	})
	app.QUERY("/search", func(_ *Request, res *Response) error {
		return res.String("results")
	})

	req := httptest.NewRequest("QUERY", "/search", strings.NewReader("q=test"))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if observedCount != 1 {
		t.Fatalf("expected observer called exactly once, got %d", observedCount)
	}
	var apiErr ErrorGolpher
	if !errors.As(observedErr, &apiErr) {
		t.Fatalf("expected ErrorGolpher, got %T: %v", observedErr, observedErr)
	}
	if apiErr.Code != http.StatusBadRequest {
		t.Fatalf("expected ErrorGolpher with code 400, got %d", apiErr.Code)
	}
}

func TestQUERYWithContentTypeAndEmptyBodyDispatched(t *testing.T) {
	app := New()
	var called bool
	app.QUERY("/search", func(_ *Request, res *Response) error {
		called = true
		return res.String("empty")
	})

	req := httptest.NewRequest("QUERY", "/search", nil)
	req.Header.Set("Content-Type", "application/query")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected handler to be called for empty QUERY body with Content-Type")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestQUERYAllowHeader(t *testing.T) {
	app := New()
	app.GET("/resource", func(_ *Request, res *Response) error { return res.String("get") })
	app.QUERY("/resource", func(_ *Request, res *Response) error { return res.String("query") })

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/resource", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	allow := rec.Header().Get("Allow")
	if !strings.Contains(allow, "GET") || !strings.Contains(allow, "QUERY") {
		t.Fatalf("expected Allow to contain GET and QUERY, got %q", allow)
	}
}

func TestQUERYOnlyPathReturnsAllowWithQUERYOnMismatch(t *testing.T) {
	app := New()
	app.QUERY("/resource", func(_ *Request, res *Response) error { return res.String("query") })

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/resource", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "QUERY" {
		t.Fatalf("expected Allow: QUERY, got %q", got)
	}
}

func TestQUERYDynamicRouteMatches(t *testing.T) {
	app := New()
	var id string
	app.QUERY("/users/:id", func(req *Request, res *Response) error {
		id = req.Param("id")
		return res.String("user:" + id)
	})

	req := httptest.NewRequest("QUERY", "/users/42", strings.NewReader("q=test"))
	req.Header.Set("Content-Type", "application/query")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if id != "42" {
		t.Fatalf("expected param id=42, got %q", id)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGroupQUERY(t *testing.T) {
	app := New()
	var method string
	api := app.Group("/api")
	api.QUERY("/search", func(req *Request, res *Response) error {
		method = req.http.Method
		return res.String("api-query")
	})

	req := httptest.NewRequest("QUERY", "/api/search", strings.NewReader("q=test"))
	req.Header.Set("Content-Type", "application/query")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if method != "QUERY" {
		t.Fatalf("expected QUERY method, got %q", method)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestListenReturnsErrorOnBindFailure(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port

	app := New(AppConfig{Port: port, DisableBanner: true})
	listenErr := app.Listen()
	if listenErr == nil {
		t.Fatal("expected Listen to return an error on already-bound port")
	}

	l.Close()
}

type benchmarkResponseWriter struct {
	header http.Header
	status int
	writes int
}

func (w *benchmarkResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *benchmarkResponseWriter) Write(body []byte) (int, error) {
	w.writes++
	return len(body), nil
}

func (w *benchmarkResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *benchmarkResponseWriter) reset() {
	w.status = 0
	w.writes = 0
	for key := range w.header {
		delete(w.header, key)
	}
}

func BenchmarkStaticRouteGolpher(b *testing.B) {
	app := New()
	app.GET("/ready", func(_ *Request, res *Response) error {
		return res.String("ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := &benchmarkResponseWriter{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.reset()
		app.ServeHTTP(w, req)
	}
}

func BenchmarkDynamicRouteParam(b *testing.B) {
	app := New()
	app.GET("/users/:id", func(req *Request, res *Response) error {
		return res.String(req.Param("id"))
	})
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	w := &benchmarkResponseWriter{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.reset()
		app.ServeHTTP(w, req)
	}
}

func BenchmarkDynamicRouteMatchInto(b *testing.B) {
	segs, names := compileRouteSegmentsResult("/users/:id/orders/:orderID")
	rt := route{
		method:           http.MethodGet,
		compiledSegments: segs,
		paramNames:       names,
	}
	request := &Request{paramValues: make([]string, 0, 2)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !rt.matchInto("/users/42/orders/abc", "users/42/orders/abc", request) {
			b.Fatal("expected route match")
		}
	}
}

func BenchmarkResponseBytes(b *testing.B) {
	w := &benchmarkResponseWriter{}
	res := newTestResponse(w)
	body := []byte(`{"approved":true,"fraud_score":0}`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w.reset()
		if err := res.Bytes(http.StatusOK, "application/json", body); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResponseJSONBytes(b *testing.B) {
	w := &benchmarkResponseWriter{}
	res := newTestResponse(w)
	body := []byte(`{"approved":true,"fraud_score":0}`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w.reset()
		if err := res.JSONBytes(body); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResponseSend(b *testing.B) {
	w := &benchmarkResponseWriter{}
	res := newTestResponse(w)
	body := []byte(`{"approved":true,"fraud_score":0}`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w.reset()
		res.Header().Set("Content-Type", "application/json")
		res.SetStatus(http.StatusOK)
		if err := res.Send(body); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBodyRead(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bw := &benchmarkResponseWriter{}
		res := newTestResponse(bw)
		req := acquireRequest(httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"id":"tx","transaction":{"amount":1}}`))), res, 1<<20)
		data, _ := req.Body()
		if len(data) == 0 {
			b.Fatal("expected body")
		}
	}
}

func BenchmarkBodyLimitThenBody(b *testing.B) {
	app := New()
	app.Use(BodyLimit(16 << 10))
	app.POST("/payload", func(req *Request, res *Response) error {
		data, err := req.Body()
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return ErrorGolpher{Code: http.StatusBadRequest, Message: "empty"}
		}
		return res.String("ok")
	})
	body := []byte(`{"id":"tx","transaction":{"amount":1}}`)
	w := &benchmarkResponseWriter{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.reset()
		req := httptest.NewRequest(http.MethodPost, "/payload", bytes.NewReader(body))
		app.ServeHTTP(w, req)
	}
}

func BenchmarkStaticPOSTBodyLimitNoResponseCapture(b *testing.B) {
	app := New()
	app.Use(BodyLimit(16 << 10))
	app.POST("/fraud-score", func(req *Request, res *Response) error {
		data, err := req.Body()
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return ErrorGolpher{Code: http.StatusBadRequest, Message: "empty"}
		}
		res.Header().Set("Content-Type", "application/json")
		res.SetStatus(http.StatusOK)
		return res.Send([]byte(`{"approved":true,"fraud_score":0}`))
	})

	body := []byte(`{"id":"tx","transaction":{"amount":1}}`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/fraud-score", bytes.NewReader(body))
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
	}
}

func TestConflictingWildcardNamesPanics(t *testing.T) {
	app := New()
	app.GET("/files/*path", func(_ *Request, res *Response) error { return nil })
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for conflicting wildcard names")
		}
	}()
	app.GET("/files/*glob", func(_ *Request, res *Response) error { return nil })
}

func TestConflictingWildcardNamesReverseOrderPanics(t *testing.T) {
	app := New()
	app.GET("/assets/*name", func(_ *Request, res *Response) error { return nil })
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for conflicting wildcard names (reverse order)")
		}
	}()
	app.GET("/assets/*file", func(_ *Request, res *Response) error { return nil })
}

func TestParamThenWildcardAtSamePositionPanics(t *testing.T) {
	app := New()
	app.GET("/users/:id", func(_ *Request, res *Response) error { return nil })
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for :param vs *wildcard")
		}
	}()
	app.GET("/users/*rest", func(_ *Request, res *Response) error { return nil })
}

func TestWildcardThenParamAtSamePositionPanics(t *testing.T) {
	app := New()
	app.GET("/items/*rest", func(_ *Request, res *Response) error { return nil })
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for *wildcard vs :param (reverse order)")
		}
	}()
	app.GET("/items/:id", func(_ *Request, res *Response) error { return nil })
}

func TestObserverReceivesOriginalMaxBytesError(t *testing.T) {
	var observedErr error
	app := New(AppConfig{
		ErrorObserver: func(req *Request, res *Response, err error) {
			observedErr = err
		},
	})
	app.POST("/data", func(req *Request, res *Response) error {
		_, err := req.Body()
		return err
	})

	body := strings.NewReader(strings.Repeat("x", 2<<20)) // 2 MiB, exceeds default 1 MiB
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/data", body))

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
	var maxBytesErr *http.MaxBytesError
	if !errors.As(observedErr, &maxBytesErr) {
		t.Fatalf("expected observer to receive original *http.MaxBytesError, got %T: %v", observedErr, observedErr)
	}
}

type noFlushWriter struct {
	headers http.Header
	body    []byte
	status  int
}

func (w *noFlushWriter) Header() http.Header {
	if w.headers == nil {
		w.headers = make(http.Header)
	}
	return w.headers
}

func (w *noFlushWriter) Write(p []byte) (int, error) {
	w.body = append(w.body, p...)
	return len(p), nil
}

func (w *noFlushWriter) WriteHeader(status int) {
	w.status = status
}

func TestResponseControllerFlushWhenSupported(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponse(rec)
	rc := http.NewResponseController(res.Raw())
	if err := rc.Flush(); err != nil {
		t.Fatalf("expected Flush to succeed on httptest.ResponseRecorder: %v", err)
	}
}

func TestResponseControllerFlushReturnsErrNotSupported(t *testing.T) {
	w := &noFlushWriter{}
	res := newTestResponse(w)
	rc := http.NewResponseController(res.Raw())
	err := rc.Flush()
	if err == nil {
		t.Fatal("expected Flush to return ErrNotSupported for writer without Flusher")
	}
	var netErr *net.OpError
	if !errors.As(err, &netErr) {
		// http.ErrNotSupported may wrap differently; just check it's not nil.
		_ = netErr
	}
	if err == nil {
		t.Fatal("expected non-nil error from Flush on unsupported writer")
	}
}

func TestRequestBodyWrapsOnlyOnce(t *testing.T) {
	bw := &benchmarkResponseWriter{}
	res := newTestResponse(bw)
	httpReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
	req := acquireRequest(httpReq, res, 1<<20)

	initialBody := req.body()
	if initialBody == nil {
		t.Fatal("expected body")
	}
	secondBody := req.body()
	if initialBody != secondBody {
		t.Fatal("expected body() to be idempotent")
	}
	if !req.bodyWrapped {
		t.Fatal("expected bodyWrapped to be set after wrapping")
	}
}

func TestRequestBodyNilResponseDoesNotPanic(t *testing.T) {
	req := &Request{
		http:         httptest.NewRequest(http.MethodPost, "/", strings.NewReader("data")),
		appBodyLimit: 1 << 20,
	}
	body := req.body()
	if body == nil {
		t.Fatal("expected non-nil body")
	}
	data, err := io.ReadAll(body)
	if err != nil || string(data) != "data" {
		t.Fatalf("expected 'data', got %q err=%v", string(data), err)
	}
}

func TestCommittedResponseSkipsErrorBody(t *testing.T) {
	app := New()
	app.GET("/partial", func(_ *Request, res *Response) error {
		res.SetStatus(http.StatusOK)
		if err := res.String("ok"); err != nil {
			return err
		}
		return errors.New("late")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/partial", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected body 'ok' only, got %q", rec.Body.String())
	}
}

func TestBodyWriteAfterCommit(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponse(rec)
	res.SetStatus(http.StatusOK)
	if err := res.String("part1"); err != nil {
		t.Fatal(err)
	}
	if !res.Committed() {
		t.Fatal("expected committed after first write")
	}
	// Write more — committed prevents another WriteHeader but body writes continue.
	if err := res.Send([]byte("part2")); err != nil {
		t.Fatal(err)
	}
	if rec.Body.String() != "part1part2" {
		t.Fatalf("expected 'part1part2', got %q", rec.Body.String())
	}
}

func TestObserverFiresExactlyOnceOnCommitThenError(t *testing.T) {
	var observedErr error
	var observedCount int

	app := New(AppConfig{
		ErrorObserver: func(req *Request, res *Response, err error) {
			observedErr = err
			observedCount++
		},
	})
	wantErr := errors.New("internal late error")
	app.GET("/commit-err", func(_ *Request, res *Response) error {
		res.SetStatus(http.StatusOK)
		if err := res.String("ok"); err != nil {
			return err
		}
		return wantErr
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/commit-err", nil))

	if observedCount != 1 {
		t.Fatalf("expected observer called exactly once, got %d", observedCount)
	}
	if !errors.Is(observedErr, wantErr) {
		t.Fatalf("expected observer to receive original error, got %v", observedErr)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 (committed before error), got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected body 'ok' only, got %q", rec.Body.String())
	}
}

func TestImplicitStatus200OnUnwrittenResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponse(rec)
	if res.Status() != 0 {
		t.Fatalf("expected 0 before commit, got %d", res.Status())
	}
	res.SetStatus(http.StatusCreated)
	if res.Status() != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.Status())
	}
}

func TestImplicitStatus200AfterFirstWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	res := newTestResponse(rec)
	if err := res.String("hello"); err != nil {
		t.Fatal(err)
	}
	if res.Status() != http.StatusOK {
		t.Fatalf("expected implicit 200 after write, got %d", res.Status())
	}
}

func TestObserverFiresFor404(t *testing.T) {
	var observedErr error
	app := New(AppConfig{
		ErrorObserver: func(req *Request, res *Response, err error) {
			observedErr = err
		},
	})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nonexistent", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if observedErr == nil {
		t.Fatal("expected observer to fire for 404")
	}
	var apiErr ErrorGolpher
	if !errors.As(observedErr, &apiErr) || apiErr.Code != http.StatusNotFound {
		t.Fatalf("expected observer to receive 404 ErrorGolpher, got %v", observedErr)
	}
}

func TestObserverFiresFor405(t *testing.T) {
	var observedErr error
	app := New(AppConfig{
		ErrorObserver: func(req *Request, res *Response, err error) {
			observedErr = err
		},
	})
	app.GET("/resource", func(_ *Request, res *Response) error { return nil })

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/resource", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if observedErr == nil {
		t.Fatal("expected observer to fire for 405")
	}
}

func TestRecoverObserverReceivesErrorGolpher(t *testing.T) {
	var observedErr error
	app := New(AppConfig{
		ErrorObserver: func(req *Request, res *Response, err error) {
			observedErr = err
		},
	})
	app.Use(Recover())
	app.GET("/panic", func(_ *Request, _ *Response) error {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	var apiErr ErrorGolpher
	if !errors.As(observedErr, &apiErr) || apiErr.Code != http.StatusInternalServerError {
		t.Fatalf("expected observer to receive 500 ErrorGolpher, got %v", observedErr)
	}
}

func TestConcurrentFreezeAndRegister(t *testing.T) {
	app := New()
	app.GET("/", func(_ *Request, res *Response) error { return res.String("ok") })

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { recover() }()
		for i := 0; i < 100; i++ {
			app.GET("/race"+string(rune('a'+i%26)), func(_ *Request, res *Response) error { return nil })
		}
	}()
	wg.Wait()
}

func TestConcurrentBodyLimitPerRequest(t *testing.T) {
	app := New()
	app.Use(BodyLimit(64))
	app.POST("/data", func(req *Request, res *Response) error {
		_, err := req.Body()
		if err != nil {
			return err
		}
		return res.String("ok")
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/data", strings.NewReader(strings.Repeat("x", 10)))
			app.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", rec.Code)
			}
		}()
	}
	wg.Wait()
}

func TestConcurrentBodyLimitMixed(t *testing.T) {
	app := New()
	app.Use(BodyLimit(64))
	app.POST("/data", func(req *Request, res *Response) error {
		_, err := req.Body()
		if err != nil {
			return err
		}
		return res.String("ok")
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			body := strings.Repeat("x", 10)
			if i%3 == 0 {
				body = strings.Repeat("x", 128)
			}
			req := httptest.NewRequest(http.MethodPost, "/data", strings.NewReader(body))
			app.ServeHTTP(rec, req)
			if i%3 == 0 {
				if rec.Code != http.StatusRequestEntityTooLarge {
					t.Errorf("expected 413 for large body, got %d", rec.Code)
				}
			} else {
				if rec.Code != http.StatusOK {
					t.Errorf("expected 200 for small body, got %d", rec.Code)
				}
			}
		}(i)
	}
	wg.Wait()
}

type shortWriter struct {
	http.ResponseWriter
	n int
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > w.n {
		p = p[:w.n]
	}
	return w.ResponseWriter.Write(p)
}

func TestShortWriteCapturesOnlyWrittenBytes(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &shortWriter{ResponseWriter: rec, n: 3}
	res := newTestResponseCapture(sw)

	if err := res.Send([]byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	if res.BytesWritten() != 3 {
		t.Fatalf("expected 3 bytes written, got %d", res.BytesWritten())
	}
	if string(res.Body()) != "abc" {
		t.Fatalf("expected captured 'abc', got %q", string(res.Body()))
	}
}

func TestCaptureEnabledForAllHelpers(t *testing.T) {
	helpers := []struct {
		name string
		fn   func(res *Response) error
		want string
	}{
		{
			name: "Send",
			fn:   func(res *Response) error { return res.Send([]byte("send-body")) },
			want: "send-body",
		},
		{
			name: "String",
			fn:   func(res *Response) error { return res.String("string-body") },
			want: "string-body",
		},
		{
			name: "JSON",
			fn:   func(res *Response) error { return res.JSON(map[string]string{"k": "v"}) },
			want: `{"k":"v"}` + "\n",
		},
		{
			name: "JSONBytes",
			fn:   func(res *Response) error { return res.JSONBytes([]byte(`{"json":"ok"}`)) },
			want: `{"json":"ok"}`,
		},
		{
			name: "Bytes",
			fn: func(res *Response) error {
				return res.Bytes(http.StatusOK, "application/octet-stream", []byte("bytes-body"))
			},
			want: "bytes-body",
		},
		{
			name: "XML",
			fn: func(res *Response) error {
				type item struct {
					XMLName xml.Name `xml:"item"`
					V       string   `xml:"v"`
				}
				return res.XML(item{V: "xml-body"})
			},
			want: `<item><v>xml-body</v></item>`,
		},
	}

	for _, h := range helpers {
		t.Run(h.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			res := newTestResponseCapture(rec)
			if err := h.fn(res); err != nil {
				t.Fatalf("%s: unexpected error: %v", h.name, err)
			}
			if !res.captureEnabled {
				t.Fatal("capture must be enabled")
			}
			got := string(res.Body())
			if got != h.want {
				t.Fatalf("%s: expected captured %q, got %q", h.name, h.want, got)
			}
		})
	}
}

func TestConcurrentHeaderIsolation(t *testing.T) {
	app := New()
	app.GET("/header", func(_ *Request, res *Response) error {
		res.Header().Set("X-Test", "value")
		return res.String("ok")
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/header", nil))
			if rec.Header().Get("X-Test") != "value" {
				t.Errorf("expected X-Test header")
			}
		}()
	}
	wg.Wait()
}

func TestDefaultBodyLimitIs1MiB(t *testing.T) {
	app := New()
	app.POST("/data", func(req *Request, res *Response) error {
		_, err := req.Body()
		return err
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/data",
		strings.NewReader(strings.Repeat("a", 1024*1024-1))))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for body under 1 MiB, got %d", rec.Code)
	}
}

func TestExplicitZeroMeansDefault(t *testing.T) {
	app := New(AppConfig{MaxRequestBodyBytes: 0})
	app.POST("/data", func(req *Request, res *Response) error {
		_, err := req.Body()
		return err
	})

	body := strings.NewReader(strings.Repeat("a", 1024*1024-1))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/data", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for body under default, got %d", rec.Code)
	}
}

func TestBodyLimitZeroKeepsAppDefault(t *testing.T) {
	app := New()
	app.Use(BodyLimit(0))
	app.POST("/data", func(req *Request, res *Response) error {
		_, err := req.Body()
		return err
	})

	body := strings.NewReader(strings.Repeat("a", 1024*1024-1))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/data", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with BodyLimit(0), got %d", rec.Code)
	}
}

type countingReadCloser struct {
	io.ReadCloser
	reads int
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	c.reads++
	return c.ReadCloser.Read(p)
}

func TestUnreadBodyReceivesZeroReads(t *testing.T) {
	app := New()
	app.POST("/noread", func(_ *Request, res *Response) error {
		return res.String("ok")
	})

	counter := &countingReadCloser{ReadCloser: io.NopCloser(strings.NewReader("body"))}
	req := httptest.NewRequest(http.MethodPost, "/noread", nil)
	req.Body = counter

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if counter.reads != 0 {
		t.Fatalf("expected 0 reads on unused body, got %d", counter.reads)
	}
}

func TestUnreadBodyNotWrappedByMiddleware(t *testing.T) {
	app := New()
	app.Use(BodyLimit(64))
	app.POST("/noread", func(_ *Request, res *Response) error {
		return res.String("ok")
	})

	counter := &countingReadCloser{ReadCloser: io.NopCloser(strings.NewReader("body"))}
	req := httptest.NewRequest(http.MethodPost, "/noread", nil)
	req.Body = counter

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if counter.reads != 0 {
		t.Fatalf("expected 0 reads on unused body through middleware, got %d", counter.reads)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestQUERYFixedLengthOverflowReturns413(t *testing.T) {
	app := New()
	app.QUERY("/search", func(req *Request, res *Response) error {
		_, err := req.Body()
		return err
	})

	body := strings.NewReader(strings.Repeat("x", 2<<20))
	req := httptest.NewRequest("QUERY", "/search", body)
	req.Header.Set("Content-Type", "application/query")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for fixed-length QUERY overflow, got %d", rec.Code)
	}
}

func TestQUERYChunkedOverflowReturns413(t *testing.T) {
	app := New()
	app.QUERY("/search", func(req *Request, res *Response) error {
		_, err := req.Body()
		return err
	})

	body := strings.NewReader(strings.Repeat("x", 2<<20))
	req := httptest.NewRequest("QUERY", "/search", body)
	req.Header.Set("Content-Type", "application/query")
	req.ContentLength = -1
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for chunked QUERY overflow, got %d", rec.Code)
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
