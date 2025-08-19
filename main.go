package main

import (
	"bufio"
	"fmt"
	"huskki/hub"
	"huskki/ui"
	"log"
	"os"
	"path/filepath"

	"go.bug.st/serial"
)

const (
	DEFAULT_BAUD_RATE = 115200
	LOG_DIR           = "logs"
	LOG_NAME          = "RAWLOG"
	LOG_EXT           = ".bin"
)

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
	EventHub *hub.EventHub
	Replayer *replayer
	UI       ui.UI
	Server   *server
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
		Replayer = newReplayer(replayFlags)
		go func() {
			if err := Replayer.run(EventHub); err != nil {
				log.Fatalf("couldn't run replay: %v", err)
			}
		}()
	}

	// Initialise UI
	UI, err = ui.NewDashboard()
	if err != nil {
		log.Fatalf("couldn't init dashboard: %v", err)
	}

	// Initialise Server
	Server = newServer(UI, EventHub)
	err = Server.Start(flags.Addr)
	if err != nil {
		log.Fatalf("couldn't start server: %v", err)
	}
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
