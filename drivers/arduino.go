package drivers

import (
	"bufio"
	"errors"
	"fmt"
	"huskki/config"
	"huskki/ecu"
	"huskki/events"
	"huskki/utils"
	"log"
	"os"
	"strings"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

type Arduino struct {
	*config.SerialFlags
	ecuProcessor ecu.Processor
	eventHub     *events.EventHub
	port         serial.Port
}

const (
	LOG_DIR              = "logs"
	LOG_NAME             = "RAWLOG"
	LOG_EXT              = ".bin"
	WRITE_EVERY_N_FRAMES = 100
)

var (
	badLenErr = errors.New("error data length outside range")
	badCrcErr = errors.New("error frame checksum does not match")
)

// Arduino & clones common VIDs
var preferredVIDs = map[string]bool{
	"2341": true, // Arduino
	"2A03": true, // Arduino (older)
	"1A86": true, // CH340
	"10C4": true, // CP210x
	"0403": true, // FTDI
}

var magicBytes = []byte{0xAA, 0x55}

func NewArduino(serialFlags *config.SerialFlags, ecuProcessor ecu.Processor, eventHub *events.EventHub) *Arduino {
	driver := &Arduino{
		serialFlags,
		ecuProcessor,
		eventHub,
		nil,
	}
	return driver
}

func (a *Arduino) Init() error {
	port, err := getArduinoPort(a.Port, a.BaudRate)
	if err != nil {
		return err
	}
	a.port = port
	return nil
}

func (a *Arduino) Run() error {
	filePath := utils.NextAvailableFilename(LOG_DIR, LOG_NAME, LOG_EXT)

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("couldn't open rawlog: %v", err)
	}

	defer func() { _ = file.Close() }()

	logWriter := bufio.NewWriterSize(file, 1<<20)
	defer func() { _ = logWriter.Flush() }()

	go processBinary(a.port, a.eventHub, a.ecuProcessor, logWriter)
	return nil
}

func getArduinoPort(port string, baud int) (serial.Port, error) {
	// auto-select Arduino-ish port if requested
	if port == "auto" {
		name, err := autoSelectPort()
		if err != nil {
			log.Fatalf("auto-select: %v", err)
		}
		port = name
	}
	mode := &serial.Mode{BaudRate: baud}
	serialPort, err := serial.Open(port, mode)
	if err != nil {
		log.Fatalf("couldn't open serial %s: %v", port, err)
	}
	log.Printf("connected to %s @ %d", port, baud)

	return serialPort, err
}

func autoSelectPort() (string, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return "", fmt.Errorf("enumerate ports: %w", err)
	}
	// Look for the first matching "arduino port"
	for _, p := range ports {
		if p.IsUSB && preferredVIDs[strings.ToUpper(p.VID)] {
			return p.Name, nil
		}
	}
	return "", fmt.Errorf("no arduino serial ports found")
}
