package web

import (
	"html/template"
	"net/http"

	ds "github.com/starfederation/datastar-go/datastar"
)

type Renderer interface {
	Templates() *template.Template
	Handlers() map[string]func(r http.ResponseWriter, w *http.Request)
	Data() map[string]interface{}
	OnTick(sse *ds.ServerSentEventGenerator, currentTimeMs int) error
}
