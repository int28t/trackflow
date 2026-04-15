package gateway

import "net/http"

type AppHandler func(http.ResponseWriter, *http.Request) error

type AppErrorHandler interface {
	Handle(http.ResponseWriter, *http.Request, error)
}

func Adapt(handler AppHandler, errorHandler AppErrorHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := handler(w, r); err != nil {
			errorHandler.Handle(w, r, err)
		}
	})
}
