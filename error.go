package golpher

import (
	"errors"
	"net/http"
)

// ErrorGolpher is the framework's structured error type.
// Handlers can return it to control the HTTP status code and
// response body. The Message field is sent to the client exactly
// as-is; the Code field maps to the HTTP status line.
type ErrorGolpher struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e ErrorGolpher) Error() string {
	return e.Message
}

// ErrorHandlerFunc receives the request and response directly after
// the observer runs. It is responsible for rendering the error
// response to the client. If the response is already committed,
// the framework skips calling this function.
type ErrorHandlerFunc func(req *Request, res *Response, err error)

// ErrorObserverFunc is called exactly once per request error,
// before the ErrorHandler runs. It receives the original error
// (not masked), even when the response has already been committed.
// If nil, no observer is invoked.
type ErrorObserverFunc func(req *Request, res *Response, err error)

// defaultErrorHandler writes the error to the client as JSON.
//
// For ErrorGolpher errors the full Code + Message payload is
// serialized. For all other errors (unknown errors) the body is
// a generic "Internal Server Error" message; the original error
// text never reaches the client.
func defaultErrorHandler(req *Request, res *Response, err error) {
	var apiErr ErrorGolpher
	if errors.As(err, &apiErr) {
		_ = res.SetStatus(apiErr.Code).JSON(apiErr)
		return
	}
	_ = res.SetStatus(http.StatusInternalServerError).JSON(ErrorGolpher{
		Code:    http.StatusInternalServerError,
		Message: http.StatusText(http.StatusInternalServerError),
	})
}

// reportError fires the observer and, if the response is not yet
// committed, delegates to the error handler.
func (app *App) reportError(req *Request, res *Response, err error) {
	if app.errorObserver != nil {
		app.errorObserver(req, res, err)
	}
	if !res.Committed() {
		app.errorHandler(req, res, err)
	}
}

// handleMaxBytesError checks whether err wraps an
// *http.MaxBytesError. If so, the observer receives the original
// error (preserving errors.As) and the error handler receives a 413
// ErrorGolpher. Returns true if the error was handled.
func (app *App) handleMaxBytesError(req *Request, res *Response, err error) bool {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		if app.errorObserver != nil {
			app.errorObserver(req, res, err)
		}
		if !res.Committed() {
			app.errorHandler(req, res, ErrorGolpher{
				Code:    http.StatusRequestEntityTooLarge,
				Message: http.StatusText(http.StatusRequestEntityTooLarge),
			})
		}
		return true
	}
	return false
}
