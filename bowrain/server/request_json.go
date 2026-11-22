package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"
)

// requestJSONSerializer is the server's echo.JSONSerializer. Responses are
// written by Echo's default serializer. A request body that c.Bind reads as
// JSON is decoded with a streaming json.Decoder: the first JSON value is
// decoded and anything after it is ignored. Handlers return the text of a
// decode error in their 400 responses, so the error text is part of the API:
//
//	code=400, message=Syntax error: offset=<n>, error=<decoder error>, internal=<decoder error>
//	code=400, message=Unmarshal type error: expected=<type>, got=<value>, field=<field>, offset=<n>, internal=<decoder error>
//	code=400, message=<decoder error>, internal=<decoder error>
//	code=413, message=Request Entity Too Large
type requestJSONSerializer struct {
	echo.DefaultJSONSerializer
}

// Deserialize decodes the request body into target.
func (requestJSONSerializer) Deserialize(c *echo.Context, target any) error {
	err := json.NewDecoder(c.Request().Body).Decode(target)
	if err == nil {
		return nil
	}
	if ute, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return newBodyError(http.StatusBadRequest,
			fmt.Sprintf("Unmarshal type error: expected=%v, got=%v, field=%v, offset=%v", ute.Type, ute.Value, ute.Field, ute.Offset), err)
	}
	if se, ok := errors.AsType[*json.SyntaxError](err); ok {
		return newBodyError(http.StatusBadRequest,
			fmt.Sprintf("Syntax error: offset=%v, error=%v", se.Offset, se.Error()), err)
	}
	// A read error with a status of its own: the body limit's 413.
	if sc, ok := err.(echo.HTTPStatusCoder); ok {
		return newBodyError(sc.StatusCode(), http.StatusText(sc.StatusCode()), nil)
	}
	return newBodyError(http.StatusBadRequest, err.Error(), err)
}

// bodyError is a request body the server could not decode. It carries a
// status, so Echo's binder returns it unchanged, and it unwraps to the
// *echo.HTTPError that httpErrorHandler reads status, message and detail from.
type bodyError struct {
	he *echo.HTTPError
}

func newBodyError(code int, message string, internal error) *bodyError {
	he := echo.NewHTTPError(code, message)
	if internal != nil {
		// Wrap returns the wrapped copy as an error; it is an *echo.HTTPError.
		if wrapped, ok := errors.AsType[*echo.HTTPError](he.Wrap(internal)); ok {
			he = wrapped
		}
	}
	return &bodyError{he: he}
}

func (e *bodyError) Error() string {
	if internal := e.he.Unwrap(); internal != nil {
		return fmt.Sprintf("code=%d, message=%v, internal=%v", e.he.Code, e.he.Message, internal)
	}
	return fmt.Sprintf("code=%d, message=%v", e.he.Code, e.he.Message)
}

// StatusCode is the HTTP status of the response the error asks for.
func (e *bodyError) StatusCode() int { return e.he.Code }

func (e *bodyError) Unwrap() error { return e.he }
