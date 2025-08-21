package ui

import (
	"html/template"
	"huskki/hub"
	"net/http"

	ds "github.com/starfederation/datastar-go/datastar"
)

type UI interface {
	Templates() *template.Template
	Handlers() map[string]func(r http.ResponseWriter, w *http.Request)
	Data() map[string]interface{}
	GeneratePatchOnEvent(event *hub.Event) func(*ds.ServerSentEventGenerator) error
	OnTick(sse *ds.ServerSentEventGenerator, currentTimeMs int) error
}
