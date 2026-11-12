// Package response writes the API's JSON error envelope:
//
//	{"error": {"code": "invalid_argument", "message": "...", "request_id": "..."}}
package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/requestid"
)

const (
	CodeInvalidArgument  = "invalid_argument"
	CodeNotFound         = "not_found"
	CodeConflict         = "conflict"
	CodeUnauthorized     = "unauthorized"
	CodeForbidden        = "forbidden"
	CodeMethodNotAllowed = "method_not_allowed"
	CodePayloadTooLarge  = "payload_too_large"
	CodeTooManyRequests  = "too_many_requests"
	CodeUnavailable      = "unavailable"
	CodeInternal         = "internal"
)

// ErrorBody is the error envelope.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail describes one error.
type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// Error maps err to a status code and envelope and aborts the request. The
// error is attached to the gin context so the access log records it; for
// errors that are not domain errors the client only sees a generic message.
func Error(c *gin.Context, err error) {
	_ = c.Error(err)
	status, code, msg := classify(err)
	Abort(c, status, code, msg)
}

// Abort writes an envelope with an explicit status and code.
func Abort(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorBody{Error: ErrorDetail{
		Code:      code,
		Message:   message,
		RequestID: requestid.From(c.Request.Context()),
	}})
}

func classify(err error) (status int, code, message string) {
	var de *domain.Error
	if !errors.As(err, &de) {
		return http.StatusInternalServerError, CodeInternal, "internal server error"
	}
	switch {
	case errors.Is(de.Kind, domain.ErrInvalidArgument):
		return http.StatusBadRequest, CodeInvalidArgument, de.Message
	case errors.Is(de.Kind, domain.ErrNotFound):
		return http.StatusNotFound, CodeNotFound, de.Message
	case errors.Is(de.Kind, domain.ErrConflict):
		return http.StatusConflict, CodeConflict, de.Message
	case errors.Is(de.Kind, domain.ErrUnauthorized):
		return http.StatusUnauthorized, CodeUnauthorized, de.Message
	case errors.Is(de.Kind, domain.ErrForbidden):
		return http.StatusForbidden, CodeForbidden, de.Message
	case errors.Is(de.Kind, domain.ErrUnavailable):
		return http.StatusServiceUnavailable, CodeUnavailable, de.Message
	default:
		return http.StatusInternalServerError, CodeInternal, "internal server error"
	}
}
