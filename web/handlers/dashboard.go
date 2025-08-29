package web

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"

	ds "github.com/starfederation/datastar-go/datastar"
	"huskki/models"
	"huskki/store"
)

type Dashboard struct {
	templates *template.Template

	chartsByStreamKey map[string]*models.Chart

	mu            sync.Mutex
	activeStreams map[string]map[string]int // clientID -> chartKey -> stream index
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
func (d *Dashboard) OnTick(sse *ds.ServerSentEventGenerator, currentTimeMs int, clientID string) error {
	writer := strings.Builder{}

	for _, stream := range store.DashboardStreams {
		chart, ok := d.ChartsByStreamKey()[stream.Key()]
		if !ok {
			// Just means we aren't displaying this stream atm.
			continue
		}

		activeIdx := d.activeStreamIndex(clientID, chart)
		if chart.Streams()[activeIdx].Key() == stream.Key() {
			chartCopy := d.chartCopyForClient(clientID, chart)
			if err := d.templates.ExecuteTemplate(&writer, "activeStream.value", chartCopy); err != nil {
				log.Printf("error executing activeStream.value template: %s", err)
			}
		}
		if err := sse.ExecuteScript(buildSparklineUpdateFunction(stream)); err != nil {
			log.Printf("error executing sparkline update function: %s", err)
		}
	}

	if writer.String() != "" {
		if err := sse.PatchElements(writer.String()); err != nil {
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

	clientIdentifier := getClientID(w, r)

	// Find the stream by key
	c := store.DashboardCharts[sig.Chart.Key]
	if c == nil || len(c.Streams()) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	idx := d.activeStreamIndex(clientIdentifier, c)
	idx = (idx + 1) % len(c.Streams())
	d.setActiveStreamIndex(clientIdentifier, c, idx)
	activeStreamKey := c.Streams()[idx].Key()

	chartCopy := d.chartCopyForClient(clientIdentifier, c)

	var buf strings.Builder
	err := d.templates.ExecuteTemplate(&buf, "activeStream.title", chartCopy)
	if err != nil {
		log.Printf("couldn't execute active stream title template %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
	err = d.templates.ExecuteTemplate(&buf, "activeStream.value", chartCopy)
	if err != nil {
		log.Printf("couldn't execute active stream value template %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
	err = d.templates.ExecuteTemplate(&buf, "activeStream.unit", chartCopy)
	if err != nil {
		log.Printf("couldn't execute active stream unit template %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	sse := ds.NewSSE(w, r)

	// Sparkline
	if err = sse.ExecuteScript(buildSparklineCycleFunction(c.Key(), activeStreamKey)); err != nil {
		log.Printf("error executing sparkline cycle function: %s", err)
	}

	// Patch elements
	if buf.String() != "" {
		_ = sse.PatchElements(buf.String()) // morphs the target element by ID
	}
}

func buildSparklineUpdateFunction(stream *models.Stream) string {
	pointMapString := "{"
	for _, point := range stream.SvgPoints() {
		pointMapString += fmt.Sprintf("%d:%v,", point.Timestamp(), point.Value())
	}
	pointMapString += "}"
	funcString := fmt.Sprintf(`s('%s','%d','%d',%s)`, stream.Key(), stream.LeftX(), stream.RightX(), pointMapString)
	return funcString
}

func buildSparklineCycleFunction(chartKey string, activeStreamKey string) string {
	funcString := fmt.Sprintf(`b('%s','%s')`, chartKey, activeStreamKey)
	return funcString
}

func (d *Dashboard) activeStreamIndex(clientID string, c *models.Chart) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.activeStreams == nil {
		d.activeStreams = make(map[string]map[string]int)
	}
	if _, ok := d.activeStreams[clientID]; !ok {
		d.activeStreams[clientID] = make(map[string]int)
	}
	if idx, ok := d.activeStreams[clientID][c.Key()]; ok {
		return idx
	}
	for i, s := range c.Streams() {
		if s.IsActive {
			d.activeStreams[clientID][c.Key()] = i
			return i
		}
	}
	d.activeStreams[clientID][c.Key()] = 0
	return 0
}

func (d *Dashboard) setActiveStreamIndex(clientID string, c *models.Chart, idx int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.activeStreams == nil {
		d.activeStreams = make(map[string]map[string]int)
	}
	if _, ok := d.activeStreams[clientID]; !ok {
		d.activeStreams[clientID] = make(map[string]int)
	}
	d.activeStreams[clientID][c.Key()] = idx
}

func (d *Dashboard) chartCopyForClient(clientID string, c *models.Chart) *models.Chart {
	idx := d.activeStreamIndex(clientID, c)
	streams := make([]*models.Stream, len(c.Streams()))
	for i, s := range c.Streams() {
		copy := *s
		copy.IsActive = i == idx
		streams[i] = &copy
	}
	return models.NewChart(c.Key(), streams, c.LayoutPriority())
}
