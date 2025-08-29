package web

import (
	"log"
	"net/http"
	"sync"
	"time"

	ds "github.com/starfederation/datastar-go/datastar"
	"huskki/store"
	"huskki/web"
)

type client struct {
	id  string
	sse *ds.ServerSentEventGenerator
}

type Server struct {
	renderer Renderer
	handler  *http.ServeMux
	mu       sync.Mutex
	clients  map[*client]struct{}
}

func NewServer(renderer Renderer) *Server {
	s := &Server{
		renderer: renderer,
		clients:  make(map[*client]struct{}),
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
	go s.tickLoop()
	log.Printf("listening on %s …", addr)
	return http.ListenAndServe(addr, s.handler)
}

// IndexHandler is the main entrypoint for the UI
func (s *Server) IndexHandler(w http.ResponseWriter, r *http.Request) {
	clientIdentifier := generateClientID()
	data := s.renderer.Data()
	data["clientID"] = clientIdentifier

	err := s.renderer.Templates().ExecuteTemplate(w, "index", data)
	if err != nil {
		log.Printf("couldn't execute template for index %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *Server) TickHandler(w http.ResponseWriter, r *http.Request) {
	clientIdentifier := r.URL.Query().Get("client")
	if clientIdentifier == "" {
		clientIdentifier = generateClientID()
	}
	sse := ds.NewSSE(w, r)
	c := &client{id: clientIdentifier, sse: sse}

	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()

	<-sse.Context().Done()

	s.mu.Lock()
	delete(s.clients, c)
	s.mu.Unlock()
}

func (s *Server) tickLoop() {
	ticker := time.NewTicker(1000 / store.DASHBOARD_FRAMERATE * time.Millisecond)
	defer ticker.Stop()
	for tick := range ticker.C {
		currentMs := int(tick.UnixMilli())

		for _, stream := range store.DashboardStreams {
			stream.OnTick(currentMs)
		}

		s.mu.Lock()
		for c := range s.clients {
			if err := s.renderer.OnTick(c.sse, currentMs, c.id); err != nil {
				log.Printf("error running renderer on tick: %s", err)
			}
		}
		s.mu.Unlock()

		for _, stream := range store.DashboardStreams {
			stream.ClearStream()
		}
	}
}
