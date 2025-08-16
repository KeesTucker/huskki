package main

import (
	"bufio"
	"errors"
	"fmt"
	"huskki/hub"
	"io"
	"log"
	"strings"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// Frame is one validated can bus frame from the stream/log.
type Frame struct {
	Millis uint32 // LE from hdr[0..3]
	DID    uint16 // BE from hdr[4..5]
	Data   []byte // len = hdr[6]
}

const WRITE_EVERY_N_FRAMES = 100

var (
	badLenErr = errors.New("error data length outside range")
	badCrcErr = errors.New("error frame checksum does not match")
)

var magicBytes = []byte{0xAA, 0x55}

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

// readBinary consumes binary can frames with layout:
// [AA 55][millis:u32 LE][DID:u16 BE][len:u8][data:len][crc8:u8]
func readBinary(reader io.Reader, eventHub *hub.EventHub, raw *bufio.Writer) {
	bufferReader := bufio.NewReader(reader)
	frames := 0

	for {
		frame, err := readOneFrame(bufferReader)
		if err != nil {
			if err != io.EOF {
				log.Printf("read frame: %v", err)
				continue
			}
			return
		}

		// Save the entire frame including crc and magic bytes, this lets us replay with the same logic
		// We could probably just save it on read but this way we have a bit more control over what data gets logged
		if raw != nil {
			// rebuild exact record
			dl := len(frame.Data)
			rec := make([]byte, 2+7+dl+1)
			rec[0], rec[1] = 0xAA, 0x55

			// header
			m := frame.Millis
			rec[2] = byte(m)
			rec[3] = byte(m >> 8)
			rec[4] = byte(m >> 16)
			rec[5] = byte(m >> 24)
			rec[6] = byte(frame.DID >> 8)
			rec[7] = byte(frame.DID)
			rec[8] = byte(dl)

			// payload
			copy(rec[9:9+dl], frame.Data)

			// crc
			crc := crc8UpdateBuf(0x00, rec[2:6])  // millis
			crc = crc8Update(crc, rec[6])         // did hi
			crc = crc8Update(crc, rec[7])         // did lo
			crc = crc8Update(crc, rec[8])         // len
			crc = crc8UpdateBuf(crc, rec[9:9+dl]) // payload
			rec[9+dl] = crc

			if _, err := raw.Write(rec); err != nil {
				log.Printf("raw write: %v", err)
			} else {
				frames++
				if (frames % WRITE_EVERY_N_FRAMES) == 0 {
					_ = raw.Flush()
				}
			}
		}

		// broadcast the frames via eventhub
		BroadcastParsedSensorData(eventHub, uint64(frame.DID), frame.Data, int(time.Now().UnixMilli()))
	}
}

// readOneFrame reads a single frame with layout:
// [AA 55][millis:u32 LE][DID:u16 BE][len:u8][data:len][crc8]
func readOneFrame(bufferReader *bufio.Reader) (Frame, error) {
	var frame Frame

	// resync on magic AA 55
	for {
		firstByte, err := bufferReader.ReadByte()
		if err != nil {
			return frame, err
		}
		if firstByte != magicBytes[0] {
			continue
		}
		secondByte, err := bufferReader.ReadByte()
		if err != nil {
			return frame, err
		}
		if secondByte == magicBytes[1] {
			break
		}
		// otherwise keep scanning
	}

	// header: millis(4 LE) + did(2 BE) + len(1)
	header := make([]byte, 7)
	if _, err := io.ReadFull(bufferReader, header); err != nil {
		return frame, err
	}
	dataLength := int(header[6])
	if dataLength < 0 || dataLength > 64 {
		return frame, fmt.Errorf("error data length %d: %w", dataLength, badLenErr)
	}

	// payload + crc
	tail := make([]byte, dataLength+1)
	if _, err := io.ReadFull(bufferReader, tail); err != nil {
		return frame, err
	}
	data := tail[:dataLength]
	crcRx := tail[dataLength]

	// verify CRC over: millis(4) + did_hi + did_lo + len + data
	crc := crc8UpdateBuf(0x00, header[:4]) // millis
	crc = crc8Update(crc, header[4])       // did hi
	crc = crc8Update(crc, header[5])       // did lo
	crc = crc8Update(crc, header[6])       // len
	crc = crc8UpdateBuf(crc, data)         // payload
	if crc != crcRx {
		return frame, badCrcErr
	}

	// parse fields
	millis := uint32(header[0]) |
		uint32(header[1])<<8 |
		uint32(header[2])<<16 |
		uint32(header[3])<<24
	did := uint16(header[4])<<8 | uint16(header[5])

	return Frame{
		Millis: millis,
		DID:    did,
		Data:   append([]byte(nil), data...), // copy
	}, nil
}

// CRC-8-CCITT helpers (poly 0x07, init 0x00)
func crc8Update(crc, b byte) byte {
	crc ^= b
	for i := 0; i < 8; i++ {
		if crc&0x80 != 0 {
			crc = (crc << 1) ^ 0x07
		} else {
			crc <<= 1
		}
	}
	return crc
}
func crc8UpdateBuf(crc byte, buffer []byte) byte {
	for _, b := range buffer {
		crc = crc8Update(crc, b)
	}
	return crc
}
