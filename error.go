package golpher

import (
	"errors"
	"log"
	"net/http"
)

type ErrorGolpher struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ErrorHandlerFunc func(ctx *Context, err error)

func (e ErrorGolpher) Error() string {
	return e.Message
}

func (ctx *Context) NewError(status int, err string) error {
	return ErrorGolpher{
		Code:    status,
		Message: err,
	}
}

func defaultErrorHandler(ctx *Context, err error) {
	var apiErr ErrorGolpher
	if errors.As(err, &apiErr) {
		if jsonErr := ctx.Response.Status(apiErr.Code).JSON(apiErr); jsonErr != nil {
			log.Println(jsonErr)
		}
	} else {
		if jsonErr := ctx.Response.Status(http.StatusInternalServerError).JSON(ErrorGolpher{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		}); jsonErr != nil {
			log.Println(jsonErr)
		}
	}
}
