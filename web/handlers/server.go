package web

import (
	"huskki/store"
	"huskki/web"
	"log"
	"net/http"
	"time"

	ds "github.com/starfederation/datastar-go/datastar"
)

type Server struct {
	renderer Renderer
	handler  *http.ServeMux
}

func NewServer(renderer Renderer) *Server {
	s := &Server{
		renderer: renderer,
	}

	handler := http.NewServeMux()
	handler.HandleFunc("/", s.IndexHandler)
	handler.HandleFunc("/tick", s.TickHandler)
	handler.Handle("/static/", http.FileServer(http.FS(web.Static)))

	for path, uiHandler := range renderer.Handlers() {
		handler.HandleFunc(path, uiHandler)
	}

	s.handler = handler

	return s
}

func (s *Server) Start(addr string) error {
	log.Printf("listening on %s …", addr)
	return http.ListenAndServe(addr, s.handler)
}

// IndexHandler is the main entrypoint for the UI
func (s *Server) IndexHandler(w http.ResponseWriter, _ *http.Request) {
	err := s.renderer.Templates().ExecuteTemplate(w, "index", s.renderer.Data())
	if err != nil {
		log.Printf("couldn't execute template for index %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *Server) TickHandler(w http.ResponseWriter, r *http.Request) {
	sse := ds.NewSSE(w, r)

	ctx := r.Context()
	ticker := time.NewTicker(1000 / store.DASHBOARD_FRAMERATE * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case tick := <-ticker.C:
			err := s.renderer.OnTick(sse, int(tick.UnixMilli()))
			if err != nil {
				log.Printf("error running renderer on tick: %s", err)
				return
			}
		}
	}
}
