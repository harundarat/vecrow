// Package handler holds the HTTP handlers. Handlers translate between HTTP
// and use cases only: bind and validate the request shape, call the use case
// through an interface, and write the response.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/harundarat/vecrow/backend/internal/delivery/http/response"
	"github.com/harundarat/vecrow/backend/internal/domain"
)

// bindJSON decodes the body into dst and validates its `binding` tags. On
// failure it writes the error response and returns false.
func bindJSON(c *gin.Context, dst any) bool {
	err := c.ShouldBindJSON(dst)
	if err == nil {
		return true
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		_ = c.Error(err)
		response.Abort(c, http.StatusRequestEntityTooLarge, response.CodePayloadTooLarge,
			fmt.Sprintf("request body must not exceed %d bytes", tooLarge.Limit))
		return false
	}
	response.Error(c, domain.InvalidArgument("%s", describeBindError(err)))
	return false
}

func describeBindError(err error) string {
	var (
		verrs     validator.ValidationErrors
		syntaxErr *json.SyntaxError
		typeErr   *json.UnmarshalTypeError
	)
	switch {
	case errors.Is(err, io.EOF):
		return "request body is required"
	case errors.As(err, &verrs):
		msgs := make([]string, len(verrs))
		for i, fe := range verrs {
			if fe.Tag() == "required" {
				msgs[i] = fe.Field() + " is required"
			} else {
				msgs[i] = fe.Field() + " is invalid"
			}
		}
		return strings.Join(msgs, "; ")
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		return "request body is not valid JSON"
	case errors.As(err, &typeErr):
		if typeErr.Field == "" {
			return "request body must be a JSON object"
		}
		return fmt.Sprintf("%s must be a %s", typeErr.Field, typeErr.Type)
	default:
		return "request body is invalid"
	}
}
