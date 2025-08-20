package ui

import (
	"html/template"
	"huskki/config"
	"huskki/hub"
	"huskki/ui/ui-components"
	"log"
	"net/http"
	"strings"

	ds "github.com/starfederation/datastar-go/datastar"
)

type Dashboard struct {
	eventHub  *hub.EventHub
	templates *template.Template

	chartsByStreamKey map[string]*ui_components.Chart
}

type chartKeySig struct {
	Chart struct {
		Key string `json:"key"`
	} `json:"chart"`
}

func NewDashboard() (dashboard *Dashboard, err error) {
	dashboard = &Dashboard{}
	templates := template.New("").Funcs(template.FuncMap{
		"ToLower": strings.ToLower,
	})
	dashboard.templates, err = templates.ParseGlob("templates/dashboard/*.gohtml")
	return dashboard, err
}

func (d *Dashboard) Templates() *template.Template {
	return d.templates
}

func (d *Dashboard) Handlers() map[string]func(w http.ResponseWriter, r *http.Request) {
	return map[string]func(w http.ResponseWriter, r *http.Request){
		"/toggle-active-stream": d.CycleStreamHandler,
	}
}

func (d *Dashboard) Data() map[string]interface{} {
	return map[string]interface{}{
		"charts": config.Charts,
	}
}

// GeneratePatchOnEvent takes an event received from the event queue, iterates the charts that are displayed on the dashboard,
// and returns a closure that can be used to patch the client.
func (d *Dashboard) GeneratePatchOnEvent(event *hub.Event) func(*ds.ServerSentEventGenerator) error {
	var writer = strings.Builder{}
	var funcs []func(generator *ds.ServerSentEventGenerator) error

	c, ok := d.ChartsByStreamKey()[event.StreamKey]
	if !ok {
		log.Printf("stream not found for stream %s", event.StreamKey)
		return nil
	}

	s, ok := config.Streams[event.StreamKey]
	if !ok {
		log.Printf("stream not found %s", event.StreamKey)
		return nil
	}

	var v float64
	switch event.Value.(type) {
	case int:
		v = float64(event.Value.(int))
	case float64:
		v = event.Value.(float64)
	default:
		log.Printf("error bad event value type %T", event.Value)
		return nil
	}

	s.Add(event.Timestamp, v)

	// Check if this is the active stream
	if c.Streams()[c.ActiveStream] == s {
		// Update stream value
		err := d.templates.ExecuteTemplate(&writer, "activeStream.value", c)
		if err != nil {
			log.Printf("error executing template: %s", err)
		}
	}

	err := d.templates.ExecuteTemplate(&writer, "chart", c)
	if err != nil {
		log.Printf("error executing template: %s", err)
	}

	// Main closure
	return func(sse *ds.ServerSentEventGenerator) error {
		// Patch UI elements
		if writer.String() != "" {
			err := sse.PatchElements(writer.String())
			if err != nil {
				return err
			}
		}

		// Exec client-side javascript
		for _, f := range funcs {
			err := f(sse)
			if err != nil {
				return err
			}
		}

		return nil
	}
}

func (d *Dashboard) ChartsByStreamKey() map[string]*ui_components.Chart {
	if d.chartsByStreamKey == nil || len(d.chartsByStreamKey) == 0 {
		d.chartsByStreamKey = make(map[string]*ui_components.Chart)
		for _, c := range config.Charts {
			for _, s := range c.Streams() {
				d.chartsByStreamKey[s.Key()] = c
			}
		}
	}

	return d.chartsByStreamKey
}

// CycleStreamHandler is called when the client clicks on a stream to switch the active stream
func (d *Dashboard) CycleStreamHandler(w http.ResponseWriter, r *http.Request) {
	// Read signals sent from the client
	var sig chartKeySig
	if err := ds.ReadSignals(r, &sig); err != nil {
		log.Printf("error reading signals: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Find the stream by key
	c := config.Charts[sig.Chart.Key]
	if c == nil || len(c.Streams()) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Cycle active stream
	c.ActiveStream = (c.ActiveStream + 1) % uint8(len(c.Streams()))

	var buf strings.Builder
	err := d.templates.ExecuteTemplate(&buf, "activeStream.title", c)
	if err != nil {
		log.Printf("couldn't execute active stream title template %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
	err = d.templates.ExecuteTemplate(&buf, "activeStream.value", c)
	if err != nil {
		log.Printf("couldn't execute active stream value template %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
	err = d.templates.ExecuteTemplate(&buf, "activeStream.unit", c)
	if err != nil {
		log.Printf("couldn't execute active stream unit template %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	sse := ds.NewSSE(w, r)
	if buf.String() != "" {
		_ = sse.PatchElements(buf.String()) // morphs the target element by ID
	}

	//TODO: swap bold line somehow
}
