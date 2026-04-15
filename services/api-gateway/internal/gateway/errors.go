package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
)

const requestIDHeader = "X-Request-ID"

type contextKey string

const requestIDContextKey contextKey = "request_id"

type HTTPError struct {
	Status  int
	Code    string
	Message string
	Err     error
}

func (e *HTTPError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}

	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
}

func (e *HTTPError) Unwrap() error {
	return e.Err
}

func NewHTTPError(status int, code, message string, err error) *HTTPError {
	return &HTTPError{
		Status:  status,
		Code:    code,
		Message: message,
		Err:     err,
	}
}

func MethodNotAllowed(method string) *HTTPError {
	return NewHTTPError(
		http.StatusMethodNotAllowed,
		"method_not_allowed",
		fmt.Sprintf("method %s is not allowed", method),
		nil,
	)
}

func NotImplemented(feature string) *HTTPError {
	return NewHTTPError(
		http.StatusNotImplemented,
		"not_implemented",
		fmt.Sprintf("%s is not implemented yet", feature),
		nil,
	)
}

type ErrorHandler struct {
	logger *log.Logger
}

func NewErrorHandler(logger *log.Logger) *ErrorHandler {
	if logger == nil {
		logger = log.Default()
	}

	return &ErrorHandler{logger: logger}
}

func (h *ErrorHandler) Handle(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "internal server error"

	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		status = httpErr.Status
		code = httpErr.Code
		message = httpErr.Message
	}

	requestID := getRequestID(r.Context())
	h.logger.Printf(
		"request failed method=%s path=%s status=%d request_id=%s err=%v",
		r.Method,
		r.URL.Path,
		status,
		requestID,
		err,
	)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if requestID != "" {
		w.Header().Set(requestIDHeader, requestID)
	}
	w.WriteHeader(status)

	response := map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	}
	if requestID != "" {
		response["request_id"] = requestID
	}

	if encodeErr := json.NewEncoder(w).Encode(response); encodeErr != nil {
		h.logger.Printf("failed to write error response: %v", encodeErr)
	}
}
