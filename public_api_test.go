package golpher_test

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-golpher/golpher"
)

func TestHandlerFuncIsOnlyExportedHandlerType(t *testing.T) {
	var _ golpher.HandlerFunc = func(req *golpher.Request, res *golpher.Response) error {
		return nil
	}
	var _ golpher.MiddlewareFunc = func(next golpher.HandlerFunc) golpher.HandlerFunc {
		return next
	}
}

func TestFromHTTPHandlerReturnsUsableHandlerFunc(t *testing.T) {
	fn := golpher.FromHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("ok"))
	}))
	golpher.New().GET("/", fn)
}

func TestFromHTTPHandlerFuncReturnsUsableHandlerFunc(t *testing.T) {
	fn := golpher.FromHTTPHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("ok"))
	})
	golpher.New().GET("/", fn)
}

func TestHandleAndVerbShorthandsAcceptHandlerFunc(t *testing.T) {
	app := golpher.New()

	h := func(req *golpher.Request, res *golpher.Response) error {
		res.SetStatus(http.StatusOK)
		return res.String("ok")
	}

	app.Handle(http.MethodGet, "/handle", h)
	app.GET("/get", h)
	app.POST("/post", h)
	app.PUT("/put", h)
	app.PATCH("/patch", h)
	app.DELETE("/delete", h)
	app.QUERY("/query", h)

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/get", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /get: expected 200, got %d", rec.Code)
	}

	req := httptest.NewRequest("QUERY", "/query", nil)
	req.Header.Set("Content-Type", "application/query")
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("QUERY /query: expected 200, got %d", rec.Code)
	}
}

func TestMethodQueryConstant(t *testing.T) {
	if golpher.MethodQuery != "QUERY" {
		t.Errorf("expected MethodQuery to be QUERY, got %q", golpher.MethodQuery)
	}
}

func TestAppIsFrozen(t *testing.T) {
	app := golpher.New()

	if app.IsFrozen() {
		t.Error("expected IsFrozen() to be false before first request")
	}

	app.GET("/", func(req *golpher.Request, res *golpher.Response) error {
		return res.String("ok")
	})
	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !app.IsFrozen() {
		t.Error("expected IsFrozen() to be true after first ServeHTTP")
	}
}

func TestListenSignatureReturnsError(t *testing.T) {
	requireListenSignature((*golpher.App).Listen)
}

func requireListenSignature(func(*golpher.App, ...golpher.ListenConfig) error) {}

func TestListenReturnsBindError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Errorf("close listener: %v", err)
		}
	})

	port := l.Addr().(*net.TCPAddr).Port
	app := golpher.New(golpher.AppConfig{Port: port, DisableBanner: true})
	listenErr := app.Listen()
	if listenErr == nil {
		t.Error("expected Listen to return an error on already-bound port")
	}
}

func TestAppConfigIsIndependent(t *testing.T) {
	cfg := golpher.AppConfig{
		ErrorHandler: func(_ *golpher.Request, res *golpher.Response, _ error) {
			_ = res.SetStatus(http.StatusTeapot).String("original")
		},
	}
	app := golpher.New(cfg)
	cfg.ErrorHandler = func(_ *golpher.Request, res *golpher.Response, _ error) {
		_ = res.SetStatus(http.StatusConflict).String("mutated")
	}

	app.GET("/", func(_ *golpher.Request, _ *golpher.Response) error {
		return errors.New("failure")
	})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusTeapot {
		t.Errorf("expected %d, got %d", http.StatusTeapot, rec.Code)
	}
	if rec.Body.String() != "original" {
		t.Errorf("expected original handler response, got %q", rec.Body.String())
	}
}

func TestGroupQUERYExternal(t *testing.T) {
	app := golpher.New()
	api := app.Group("/api")
	api.QUERY("/search", func(req *golpher.Request, res *golpher.Response) error {
		return res.String("api-query-result")
	})

	req := httptest.NewRequest("QUERY", "/api/search", strings.NewReader("q=test"))
	req.Header.Set("Content-Type", "application/query")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "api-query-result" {
		t.Errorf("expected 'api-query-result', got %q", rec.Body.String())
	}
}

func TestErrorObserverCallbackExternal(t *testing.T) {
	var observed error
	app := golpher.New(golpher.AppConfig{
		ErrorObserver: func(req *golpher.Request, res *golpher.Response, err error) {
			observed = err
		},
	})
	sentinel := errors.New("some failure")
	app.GET("/err", func(req *golpher.Request, res *golpher.Response) error {
		return sentinel
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/err", nil))

	if observed == nil {
		t.Error("expected observer to be called")
	}
	if !errors.Is(observed, sentinel) {
		t.Errorf("expected observer to receive original error, got %v", observed)
	}
}

func TestBodyJSONExternal(t *testing.T) {
	app := golpher.New()
	app.POST("/json", func(req *golpher.Request, res *golpher.Response) error {
		var v map[string]string
		if err := req.BodyJSON(&v); err != nil {
			return err
		}
		return res.String(v["name"])
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/json",
		strings.NewReader(`{"name":"golpher"}`)))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "golpher" {
		t.Errorf("expected 'golpher', got %q", rec.Body.String())
	}
}

func TestBodyXMLExternal(t *testing.T) {
	app := golpher.New()
	app.POST("/xml", func(req *golpher.Request, res *golpher.Response) error {
		var v struct {
			Name string `xml:"name"`
		}
		if err := req.BodyXML(&v); err != nil {
			return err
		}
		return res.String(v.Name)
	})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/xml",
		strings.NewReader(`<item><name>golpher</name></item>`)))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "golpher" {
		t.Errorf("expected 'golpher', got %q", rec.Body.String())
	}
}

func TestFromHTTPHandlerEndToEndExternal(t *testing.T) {
	app := golpher.New()
	app.Handle(http.MethodGet, "/mounted", golpher.FromHTTPHandler(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Mounted", "yes")
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, "mounted-ok")
		}),
	))

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mounted", nil))

	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}
	if rec.Header().Get("X-Mounted") != "yes" {
		t.Errorf("expected X-Mounted header")
	}
	if rec.Body.String() != "mounted-ok" {
		t.Errorf("expected 'mounted-ok', got %q", rec.Body.String())
	}
}
