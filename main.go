package main

import (
	"bufio"
	"embed"
	"fmt"
	"html/template"
	"huskki/hub"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.bug.st/serial"
)

const (
	DEFAULT_BAUD_RATE = 115200
	LOG_DIR           = "logs"
	LOG_NAME          = "RAWLOG"
	LOG_EXT           = ".bin"
)

//go:embed static/*
var static embed.FS

// Arduino & clones common VIDs
var preferredVIDs = map[string]bool{
	"2341": true, // Arduino
	"2A03": true, // Arduino (older)
	"1A86": true, // CH340
	"10C4": true, // CP210x
	"0403": true, // FTDI
}

// Globals
var (
	Templates *template.Template
	EventHub  *hub.EventHub
)

func main() {
	flags, replayFlags := getFlags()

	EventHub = hub.NewHub()

	isReplay := replayFlags.Path != ""

	var serialPort serial.Port
	var err error
	if !isReplay {
		serialPort, err = getArduinoPort(flags.Port, flags.BaudRate)
		defer func() {
			if err := serialPort.Close(); err != nil {
				log.Printf("close serial: %v", err)
			}
		}()
	}

	if !isReplay {
		filePath := nextAvailableFilename(LOG_DIR, LOG_NAME, LOG_EXT)

		var rawW *bufio.Writer
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("couldn't open rawlog: %v", err)
		}
		defer func() { _ = f.Close() }()
		rawW = bufio.NewWriterSize(f, 1<<20)
		defer func() { _ = rawW.Flush() }()

		go readBinary(serialPort, EventHub, rawW)
	} else {
		replayer := newReplayer(replayFlags)
		go func() {
			if err := replayer.run(EventHub); err != nil {
				log.Fatalf("couldn't run replay: %v", err)
			}
		}()
	}

	// Initialise HTML templating
	Templates = template.New("").Funcs(template.FuncMap{
		"ToLower": strings.ToLower,
	})
	Templates, err = Templates.ParseGlob("templates/*.gohtml")
	if err != nil {
		log.Fatal(err)
	}

	handler := http.NewServeMux()
	handler.HandleFunc("/", IndexHandler)
	handler.HandleFunc("/events", EventsHandler)
	handler.Handle("/static/", http.FileServer(http.FS(static)))

	handler.HandleFunc("/toggle-active-stream", ToggleActiveHandler)

	log.Printf("listening on %s …", flags.Addr)
	log.Fatal(http.ListenAndServe(flags.Addr, handler))
}

func nextAvailableFilename(dir, name, ext string) string {
	path := filepath.Join(dir, name+ext)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	for i := 1; ; i++ {
		newName := fmt.Sprintf("%s_%d%s", name, i, ext)
		newPath := filepath.Join(dir, newName)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}
}
