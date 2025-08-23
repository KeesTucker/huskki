package drivers

import (
	"bufio"
	"fmt"
	"huskki/ecus"
	"huskki/events"
	"io"
	"log"
	"time"
)

var magicBytes = []byte{0xAA, 0x55}

// processBinary consumes binary did log data with layout:
// [AA 55][millis:u32 LE][DID:u16 BE][len:u8][data:len][crc8:u8]
func processBinary(reader io.Reader, eventHub *events.EventHub, processor ecus.ECUProcessor, logWriter *bufio.Writer) {
	bufferReader := bufio.NewReader(reader)
	frames := 0

	for {
		did, value, timestamp, err := readBinaryFrame(bufferReader)
		if err != nil {
			if err != io.EOF {
				log.Printf("read frame: %v", err)
				continue
			}
			// TODO: this would be a cool place to broadcast the frame to a channel or thro an event hub to be consumed elsewhere
			return
		}

		// TODO: extract the following to a logger that consumes frames from the aforementioned event hub or channel

		// Save the entire frame including crc and magic bytes, this lets us replay with the same logic
		// We could probably just save it on read but this way we have a bit more control over what data gets logged
		if logWriter != nil {
			// rebuild exact record
			dl := len(value)
			rec := make([]byte, 2+7+dl+1)
			rec[0], rec[1] = 0xAA, 0x55

			// header
			m := timestamp
			rec[2] = byte(m)
			rec[3] = byte(m >> 8)
			rec[4] = byte(m >> 16)
			rec[5] = byte(m >> 24)
			rec[6] = byte(did >> 8)
			rec[7] = byte(did)
			rec[8] = byte(dl)

			// payload
			copy(rec[9:9+dl], value)

			// crc
			crc := crc8UpdateBuf(0x00, rec[2:6])  // millis
			crc = crc8Update(crc, rec[6])         // did hi
			crc = crc8Update(crc, rec[7])         // did lo
			crc = crc8Update(crc, rec[8])         // len
			crc = crc8UpdateBuf(crc, rec[9:9+dl]) // payload
			rec[9+dl] = crc

			if _, err := logWriter.Write(rec); err != nil {
				log.Printf("raw write: %v", err)
			} else {
				frames++
				if (frames % WRITE_EVERY_N_FRAMES) == 0 {
					_ = logWriter.Flush()
				}
			}
		}

		// broadcast the frames via eventhub
		key, didValue := processor.ParseDIDBytes(uint64(did), value)
		eventHub.Broadcast(&events.Event{StreamKey: key, Timestamp: int(time.Now().UnixMilli()), Value: didValue})
	}
}

// readBinaryFrame reads a single frame with layout:
// [AA 55][millis:u32 LE][DID:u16 BE][len:u8][data:len][crc8]
func readBinaryFrame(bufferReader *bufio.Reader) (did uint16, value []byte, timestamp uint32, err error) {

	// resync on magic AA 55
	for {
		firstByte, err := bufferReader.ReadByte()
		if err != nil {
			return 0, nil, 0, err
		}
		if firstByte != magicBytes[0] {
			continue
		}
		secondByte, err := bufferReader.ReadByte()
		if err != nil {
			return 0, nil, 0, err
		}
		if secondByte == magicBytes[1] {
			break
		}
		// otherwise keep scanning
	}

	// header: millis(4 LE) + did(2 BE) + len(1)
	header := make([]byte, 7)
	if _, err = io.ReadFull(bufferReader, header); err != nil {
		return 0, nil, 0, err
	}
	dataLength := int(header[6])
	if dataLength < 0 || dataLength > 64 {
		return 0, nil, 0, fmt.Errorf("error data length %d: %w", dataLength, badLenErr)
	}

	// payload + crc
	tail := make([]byte, dataLength+1)
	if _, err = io.ReadFull(bufferReader, tail); err != nil {
		return 0, nil, 0, err
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
		return 0, nil, 0, badCrcErr
	}

	// parse fields
	millis := uint32(header[0]) |
		uint32(header[1])<<8 |
		uint32(header[2])<<16 |
		uint32(header[3])<<24

	did = uint16(header[4])<<8 | uint16(header[5])
	value = append([]byte(nil), data...)
	timestamp = millis

	return did, data, timestamp, nil
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
