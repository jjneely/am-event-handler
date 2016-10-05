package main

import (
	"log"
	"net/http"
)

type StatusResponseWriter struct {
	http.ResponseWriter
	Status int
}

func (w *StatusResponseWriter) WriteHeader(code int) {
	w.Status = code
	w.ResponseWriter.WriteHeader(code)
}

func NewStatusResponseWriter(w http.ResponseWriter) *StatusResponseWriter {
	return &StatusResponseWriter{w, 200}
}

func logRequest(w *StatusResponseWriter, r *http.Request) {
	log.Printf("%s %s \"%s %s %s\" %d",
		r.RemoteAddr, "-", r.Method, r.RequestURI, r.Proto, w.Status)
}
