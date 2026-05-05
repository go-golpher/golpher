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
	"testing"
	"time"
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

func TestAppMethodHelpersRegisterRoutes(t *testing.T) {
	app := New()
	app.PUT("/items/:id", func(req *Request, res *Response) error {
		return res.Status(http.StatusOK).String("put:" + req.Param("id"))
	})
	app.PATCH("/items/:id", func(req *Request, res *Response) error {
		return res.Status(http.StatusOK).String("patch:" + req.Param("id"))
	})
	app.DELETE("/items/:id", func(req *Request, res *Response) error {
		return res.Status(http.StatusOK).String("delete:" + req.Param("id"))
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

func TestAppRawRegistersRouteAndDispatchesStandardHandler(t *testing.T) {
	app := New()
	app.Raw(http.MethodPost, "/raw", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/raw", nil))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", got)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Fatalf("expected raw response body, got %q", rec.Body.String())
	}
}

func TestRawStaticRoutePreservesEarlierDynamicRoutePrecedence(t *testing.T) {
	app := New()
	app.GET("/:id", func(req *Request, res *Response) error {
		return res.String("dynamic:" + req.Param("id"))
	})
	app.Raw(http.MethodGet, "/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("raw"))
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "dynamic:health" {
		t.Fatalf("expected earlier dynamic route to preserve precedence, got %q", rec.Body.String())
	}
}

func TestRawRouteBypassesGolpherMiddleware(t *testing.T) {
	app := New()
	var middlewareCalled bool
	app.Use(func(next HandlerFunc) HandlerFunc {
		return func(req *Request, res *Response) error {
			middlewareCalled = true
			return next(req, res)
		}
	})
	app.Raw(http.MethodGet, "/raw", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("raw"))
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/raw", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "raw" {
		t.Fatalf("expected raw body, got %q", rec.Body.String())
	}
	if middlewareCalled {
		t.Fatal("expected raw route to bypass Golpher middleware")
	}
}

func TestRawRouteRegisteredBeforeMiddlewareDoesNotBuildNilMiddlewareChain(t *testing.T) {
	app := New()
	app.Raw(http.MethodGet, "/raw", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("raw"))
	})

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected adding middleware after raw route not to panic, got %v", recovered)
		}
	}()
	app.Use(func(next HandlerFunc) HandlerFunc {
		if next == nil {
			panic("nil next")
		}
		return func(req *Request, res *Response) error {
			return next(req, res)
		}
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/raw", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "raw" {
		t.Fatalf("expected raw body, got %q", rec.Body.String())
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

func TestStaticRouteFastPathPreservesTrailingSlashCompatibility(t *testing.T) {
	app := New()
	app.GET("/hello", func(_ *Request, res *Response) error {
		return res.Status(http.StatusOK).String("hello")
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

func TestStaticRouteFastPathPreservesRegistrationOrderWithEarlierDynamicRoute(t *testing.T) {
	app := New()
	app.GET("/:id", func(req *Request, res *Response) error {
		return res.Status(http.StatusOK).String("dynamic:" + req.Param("id"))
	})
	app.GET("/health", func(_ *Request, res *Response) error {
		return res.Status(http.StatusOK).String("static")
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "dynamic:health" {
		t.Fatalf("expected earlier dynamic route to preserve precedence, got %q", rec.Body.String())
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

func TestUseHTTPRespectsDisabledResponseBodyCapture(t *testing.T) {
	app := New(AppConfig{DisableResponseBodyCapture: true})
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
		return res.Status(http.StatusCreated).String("post")
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

func TestBodyLimitNegativeLeavesBodyUnlimited(t *testing.T) {
	app := New()
	app.Use(BodyLimit(-1))
	app.POST("/payload", func(req *Request, res *Response) error {
		return res.Status(http.StatusOK).String(string(req.Body().Bytes()))
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

func TestBodyLimitReturnsReadError(t *testing.T) {
	expectedErr := errors.New("body read failed")
	app := New()
	app.Use(BodyLimit(16))
	app.POST("/payload", func(_ *Request, res *Response) error {
		return res.String("unreachable")
	})

	httpReq := httptest.NewRequest(http.MethodPost, "/payload", nil)
	httpReq.Body = failingReadCloser{err: expectedErr}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), expectedErr.Error()) {
		t.Fatalf("expected read error in default error response, got %q", rec.Body.String())
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

func TestResponseJSONBytesWritesPreEncodedJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	res := &Response{writer: rec}

	if err := res.Status(http.StatusCreated).JSONBytes([]byte(`{"status":"ok"}`)); err != nil {
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
	res := &Response{writer: rec}

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
	res := &Response{writer: rec}

	if err := res.Status(http.StatusAccepted).Bytes(0, "text/plain", []byte("accepted")); err != nil {
		t.Fatalf("unexpected Bytes error: %v", err)
	}

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
}

func TestResponseRawExposesUnderlyingWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	res := &Response{writer: rec}

	if res.Raw() != rec {
		t.Fatal("expected raw response writer")
	}

	res.Header().Set("X-Raw", "ok")
	if got := rec.Header().Get("X-Raw"); got != "ok" {
		t.Fatalf("expected raw header ok, got %q", got)
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

func TestResponseBodyCaptureCanBeDisabled(t *testing.T) {
	rec := httptest.NewRecorder()
	res := acquireResponse(rec, true)
	defer releaseResponse(res)

	if err := res.Status(http.StatusOK).Send([]byte("golpher")); err != nil {
		t.Fatalf("unexpected send error: %v", err)
	}

	if rec.Body.String() != "golpher" {
		t.Fatalf("expected writer body golpher, got %q", rec.Body.String())
	}
	if res.BodyString() != "" {
		t.Fatalf("expected disabled response snapshot to stay empty, got %q", res.BodyString())
	}
}

func TestPooledRequestReusesBodyWrapperAndClearsState(t *testing.T) {
	firstHTTPReq := httptest.NewRequest(http.MethodPost, "/items/1?query=old", strings.NewReader("first"))
	firstReq := acquireRequest(firstHTTPReq, map[string]string{"id": "1"})
	firstBody := firstReq.Body()

	if firstReq.Param("id") != "1" || string(firstBody.Bytes()) != "first" {
		t.Fatalf("expected first request state to be populated")
	}

	releaseRequest(firstReq)

	if firstReq.Raw() != nil || firstReq.params != nil {
		t.Fatalf("expected released request wrapper to clear references, got %#v", firstReq)
	}
	if firstReq.body != firstBody {
		t.Fatal("expected pooled request to keep body wrapper for reuse")
	}
	if firstBody.bytes != nil || firstBody.error != nil || firstBody.loaded {
		t.Fatalf("expected released body wrapper to clear state, got bytes=%q error=%v loaded=%v", string(firstBody.bytes), firstBody.error, firstBody.loaded)
	}

	secondHTTPReq := httptest.NewRequest(http.MethodPost, "/items/2?query=new", strings.NewReader("second"))
	secondReq := acquireRequest(secondHTTPReq, nil)
	defer releaseRequest(secondReq)

	if secondReq.Raw() != secondHTTPReq {
		t.Fatal("expected second request to expose its own http request")
	}
	if secondReq.Param("id") != "" {
		t.Fatalf("expected pooled request params to be cleared, got %q", secondReq.Param("id"))
	}
	if secondReq.Query("query") != "new" {
		t.Fatalf("expected second request query, got %q", secondReq.Query("query"))
	}
	if string(secondReq.Body().Bytes()) != "second" {
		t.Fatalf("expected second request body, got %q", string(secondReq.Body().Bytes()))
	}
}

func TestPooledResponseClearsStateBeforeReuse(t *testing.T) {
	firstRec := httptest.NewRecorder()
	firstRes := acquireResponse(firstRec)

	if err := firstRes.Status(http.StatusCreated).String("first"); err != nil {
		t.Fatalf("unexpected first response error: %v", err)
	}
	if firstRes.statusCode != http.StatusCreated || firstRes.BodyString() != "first" {
		t.Fatalf("expected first response state to be populated")
	}

	releaseResponse(firstRes)

	if firstRes.writer != nil || firstRes.statusCode != 0 || firstRes.BodyString() != "" {
		t.Fatalf("expected released response wrapper to clear state, got %#v", firstRes)
	}

	secondRec := httptest.NewRecorder()
	secondRes := acquireResponse(secondRec)
	defer releaseResponse(secondRes)

	if secondRes.Raw() != secondRec {
		t.Fatal("expected second response to expose its own writer")
	}
	if secondRes.statusCode != 0 || secondRes.BodyString() != "" {
		t.Fatalf("expected pooled response state to be cleared, status=%d body=%q", secondRes.statusCode, secondRes.BodyString())
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
	res := acquireResponse(rec)
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
	app := New()
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

func TestResponseRedirectWritesLocationStatusAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	res := &Response{writer: rec}

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

func TestContextNewErrorAndErrorString(t *testing.T) {
	err := (&Context{}).NewError(http.StatusConflict, "conflict")
	var apiErr ErrorGolpher
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected ErrorGolpher, got %T", err)
	}
	if apiErr.Code != http.StatusConflict || apiErr.Error() != "conflict" {
		t.Fatalf("unexpected error payload: %#v", apiErr)
	}
}

func TestDefaultErrorHandlerWritesErrorGolpherJSON(t *testing.T) {
	app := New()
	app.GET("/conflict", func(req *Request, _ *Response) error {
		return req.NewError(http.StatusConflict, "conflict")
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

func TestDefaultErrorHandlerWritesGenericInternalServerError(t *testing.T) {
	app := New()
	app.GET("/boom", func(_ *Request, _ *Response) error {
		return errors.New("boom")
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
	if payload.Code != http.StatusInternalServerError || payload.Message != "boom" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func TestCustomErrorHandlerOverridesDefaultBehavior(t *testing.T) {
	app := New(AppConfig{
		ErrorHandler: func(ctx *Context, _ error) {
			_ = ctx.Response.Status(http.StatusBadGateway).JSON(map[string]string{"error": "masked"})
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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("unexpected response body close error: %v", err)
		}
	}()

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

func BenchmarkStaticRouteRaw(b *testing.B) {
	app := New()
	app.Raw(http.MethodGet, "/ready", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
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

func BenchmarkStaticRouteGolpher(b *testing.B) {
	app := New(AppConfig{DisableResponseBodyCapture: true})
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
	app := New(AppConfig{DisableResponseBodyCapture: true})
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

func BenchmarkResponseBytes(b *testing.B) {
	w := &benchmarkResponseWriter{}
	body := []byte(`{"approved":true,"fraud_score":0}`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w.reset()
		res := &Response{writer: w}
		if err := res.Bytes(http.StatusOK, "application/json", body); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResponseSend(b *testing.B) {
	w := &benchmarkResponseWriter{}
	body := []byte(`{"approved":true,"fraud_score":0}`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w.reset()
		res := &Response{writer: w}
		res.Header().Set("Content-Type", "application/json")
		if err := res.Status(http.StatusOK).Send(body); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBodyRead(b *testing.B) {
	body := []byte(`{"id":"tx","transaction":{"amount":1}}`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := &Request{http: httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))}
		if len(req.Body().Bytes()) == 0 {
			b.Fatal("expected body")
		}
	}
}

func BenchmarkBodyLimitThenBody(b *testing.B) {
	app := New(AppConfig{DisableResponseBodyCapture: true})
	app.Use(BodyLimit(16 << 10))
	app.POST("/payload", func(req *Request, res *Response) error {
		if len(req.Body().Bytes()) == 0 {
			return req.NewError(http.StatusBadRequest, "empty")
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
	app := New(AppConfig{DisableResponseBodyCapture: true})
	app.Use(BodyLimit(16 << 10))
	app.POST("/fraud-score", func(req *Request, res *Response) error {
		if len(req.Body().Bytes()) == 0 {
			return req.NewError(http.StatusBadRequest, "empty")
		}
		res.Header().Set("Content-Type", "application/json")
		return res.Status(http.StatusOK).Send([]byte(`{"approved":true,"fraud_score":0}`))
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
