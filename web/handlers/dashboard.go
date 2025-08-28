package web

import (
	"html/template"
	"huskki/models"
	"huskki/store"
	"log"
	"net/http"
	"strings"

	ds "github.com/starfederation/datastar-go/datastar"
)

type Dashboard struct {
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
		"charts": store.OrderedCharts(),
	}
}

// OnTick updates UI that should update on a tick (charts).
func (d *Dashboard) OnTick(sse *ds.ServerSentEventGenerator, currentTimeMs int) error {
	var writer = strings.Builder{}

	for _, stream := range store.DashboardStreams {
		chart, ok := d.ChartsByStreamKey()[stream.Key()]
		if !ok {
			// Just means we aren't displaying this stream atm.
			continue
		}

		// Run on tick data events
		stream.OnTick(currentTimeMs)

		// Current Value
		if stream.IsActive {
			// Update stream value
			err := d.templates.ExecuteTemplate(&writer, "activeStream.value", chart)
			if err != nil {
				log.Printf("error executing template: %s", err)
			}
		}
		// Sparkline
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
		for _, c := range store.DashboardCharts {
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
	c := store.DashboardCharts[sig.Chart.Key]
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
