package web

import (
	"html/template"
	"huskki/config"
	"huskki/events"
	"huskki/models"
	"log"
	"net/http"
	"strings"

	ds "github.com/starfederation/datastar-go/datastar"
)

type Dashboard struct {
	eventHub  *events.EventHub
	templates *template.Template

	chartsByStreamKey map[string]*models.Chart
}

type chartKeySig struct {
	Chart struct {
		Key string `json:"key"`
	} `json:"chart"`
}

func NewDashboard() (dashboard *Dashboard, err error) {
	dashboard = &Dashboard{}
	templates := template.New("").Funcs(template.FuncMap{
		"sub":        func(a, b float64) float64 { return a - b },
		"keyToTitle": func(s string) string { return strings.Replace(s, "-", " ", -1) },
	})
	dashboard.templates, err = templates.ParseGlob("web/templates/dashboard/*.gohtml")
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
		"charts": config.DashboardCharts,
	}
}

// GeneratePatchOnEvent takes an event received from the event queue, iterates the charts that are displayed on the dashboard,
// and returns a closure that can be used to patch the client.
func (d *Dashboard) GeneratePatchOnEvent(event *events.Event) func(*ds.ServerSentEventGenerator) error {
	var writer = strings.Builder{}
	c, ok := d.ChartsByStreamKey()[event.StreamKey]
	if !ok {
		log.Printf("chart for stream not found with key: %s", event.StreamKey)
		return nil
	}

	s, ok := config.DashboardStreams[event.StreamKey]
	if !ok {
		log.Printf("stream not found with key: %s", event.StreamKey)
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

	if s.Discrete() {
		// Add point with same timestamp and the last point's value if this is discrete data so we get that nice
		// stepped look
		s.Add(event.Timestamp, s.Latest().Value())
	}

	s.Add(event.Timestamp, v)

	// Check if this is the active stream
	if s.IsActive {
		// Update stream value
		err := d.templates.ExecuteTemplate(&writer, "activeStream.value", c)
		if err != nil {
			log.Printf("error executing template: %s", err)
		}
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

		return nil
	}
}

// OnTick updates UI that should update on a tick (charts).
func (d *Dashboard) OnTick(sse *ds.ServerSentEventGenerator, currentTimeMs int) error {
	var writer = strings.Builder{}

	for _, stream := range config.DashboardStreams {
		stream.OnTick(currentTimeMs)
		if err := d.templates.ExecuteTemplate(&writer, "sparkline", stream); err != nil {
			log.Printf("error executing template: %s", err)
		}
	}

	if writer.String() != "" {
		err := sse.PatchElements(writer.String())
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *Dashboard) ChartsByStreamKey() map[string]*models.Chart {
	if d.chartsByStreamKey == nil || len(d.chartsByStreamKey) == 0 {
		d.chartsByStreamKey = make(map[string]*models.Chart)
		for _, c := range config.DashboardCharts {
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
	c := config.DashboardCharts[sig.Chart.Key]
	if c == nil || len(c.Streams()) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Cycle active stream
	for i := 0; i < len(c.Streams()); i++ {
		if c.Streams()[i].IsActive {
			// Set current stream inactive
			c.Streams()[i].IsActive = false
			// Increment by 1 and use modulo to get the remainder of (i+1) / len which conveniently lets
			// us loop from 0 -> len - 1
			indexToSetActive := (i + 1) % len(c.Streams())
			c.Streams()[indexToSetActive].IsActive = true

			break
		}
	}

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
}
