package main

import (
	"huskki/hub"
	"huskki/ui"
	"log"
	"net/http"
)

type server struct {
	ui       ui.UI
	eventHub *hub.EventHub
	handler  *http.ServeMux
}

func newServer(ui ui.UI, eventHub *hub.EventHub) *server {
	s := &server{
		ui:       ui,
		eventHub: eventHub,
	}

	handler := http.NewServeMux()
	handler.HandleFunc("/", s.IndexHandler)
	handler.HandleFunc("/events", s.EventsHandler)
	handler.HandleFunc("/time", s.TimeHandler)
	handler.Handle("/static/", http.FileServer(http.FS(static)))

	for path, uiHandler := range ui.Handlers() {
		handler.HandleFunc(path, uiHandler)
	}

	s.handler = handler

	return s
}

func (s *server) Start(addr string) error {
	log.Printf("listening on %s …", addr)
	return http.ListenAndServe(addr, s.handler)
}
