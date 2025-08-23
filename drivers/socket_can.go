package drivers

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"huskki/config"
	"huskki/ecus"
	"huskki/store"
	"huskki/utils"
	"io"
	"log"
	"os"
	"time"

	"go.einride.tech/can"
	"go.einride.tech/can/pkg/socketcan"
)

const (
	CanNetwork = "can"

	CanIdReq = 0x7E0
	CanIdRsp = 0x7E8

	SidTesterPresent        = 0x3E
	SidSecurityAccess       = 0x27
	SidReadDataByIdentifier = 0x22
	PosOffset               = 0x40

	SaL2RequestSeed = 0x03
	SaL2SendKey     = 0x04
	SaL3RequestSeed = 0x05
	SaL3SendKey     = 0x06

	TesterPresentPeriodMs = 2000

	MinDidGap = 50 * time.Millisecond
)

var fastDIDs = []uint16{
	0x0100, // RPM (raw/4)
	0x0009, // Coolant °C
	0x0076, // TPS (0..1023)
	0x0070, // Grip (0..255)
	0x0001, // Throttle (0..255)
	0x0031, // Gear enum
	0x0110, // Injection Time Cyl #1
}

// var slowDIDs = []uint16{ ... }

// ===== Driver =====

type SocketCAN struct {
	*config.SocketCANFlags
	ecuProcessor ecus.ECUProcessor
	recv         *socketcan.Receiver
	tx           *socketcan.Transmitter

	// runtime state
	startTime time.Time
	lastTP    time.Time
	index     int
	lastChk   []byte
	lastLen   []byte
	lastRead  []time.Time
	writer    io.Writer
	conn      io.ReadWriteCloser
}

func NewSocketCAN(socketCANFlags *config.SocketCANFlags, ecuProcessor ecus.ECUProcessor) *SocketCAN {
	return &SocketCAN{
		SocketCANFlags: socketCANFlags,
		ecuProcessor:   ecuProcessor,
	}
}

func (p *SocketCAN) Init() error {
	ctx := context.Background()
	conn, err := socketcan.DialContext(ctx, CanNetwork, p.SocketCanAddr)
	if err != nil {
		log.Printf("socketCAN: failed to connect to %s: %s", p.SocketCanAddr, err)
		return err
	}
	p.conn = conn
	p.recv = socketcan.NewReceiver(conn)
	p.tx = socketcan.NewTransmitter(conn)

	// set up logging target identical to Arduino driver
	filePath := utils.NextAvailableFilename(LOG_DIR, LOG_NAME, LOG_EXT)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("couldn't open rawlog: %w", err)
	}
	// buffered writer; will be flushed in Run loop on close or periodically
	p.writer = bufio.NewWriterSize(file, 1<<20) // 1MB buffer

	// init state
	nFast := len(fastDIDs)
	p.lastChk = make([]byte, nFast)
	p.lastLen = make([]byte, nFast)
	p.lastRead = make([]time.Time, nFast) // new

	p.startTime = time.Now()
	now := time.Now()
	p.lastTP = now

	err = p.securityAccessLevel(3)
	if err != nil {
		return err
	}

	return nil
}

func (p *SocketCAN) Run() error {
	go p.TesterPresent(context.Background())

	flushTicker := time.NewTicker(2 * time.Second)
	defer flushTicker.Stop()

	p.index = 0
	for {
		now := time.Now()

		p.index = (p.index + 1) % len(fastDIDs)

		idx := p.index
		did := fastDIDs[idx]

		// per-DID rate limit: skip if last successful/attempted read < 10ms ago
		if !p.lastRead[idx].IsZero() && now.Sub(p.lastRead[idx]) < MinDidGap {
			p.index = (p.index + 1) % len(fastDIDs)
			time.Sleep(10 * time.Millisecond)
			continue
		}

		data, err := p.readDID(did)
		p.lastRead[idx] = now
		if err != nil {
			log.Printf("failed to read DID: %s", err)
			continue
		}

		if len(data) == 0 {
			continue
		}

		var chk byte
		for _, b := range data {
			chk ^= b
		}
		changed := (chk != p.lastChk[idx]) || (byte(len(data)) != p.lastLen[idx])
		if changed {
			key, didValue := p.ecuProcessor.ParseDIDBytes(uint64(did), data)
			if key != "" {
				stream, ok := store.DashboardStreams[key]
				if ok {
					if stream.Discrete() {
						// Add point with same timestamp and the last point's value if this is discrete data so we get that nice
						// stepped look
						stream.Add(int(now.UnixMilli()), didValue)
					}

					stream.Add(int(now.UnixMilli()), didValue)
				}
			}

			_ = p.writeFrame(did, data)
			p.lastChk[idx] = chk
			p.lastLen[idx] = byte(len(data))
		}

		// Periodic flush.
		select {
		case <-flushTicker.C:
			if bw, ok := p.writer.(*bufio.Writer); ok {
				_ = bw.Flush()
			}
		default:
		}
	}
}

func (p *SocketCAN) TesterPresent(ctx context.Context) {
	for {
		// Keep-alive (doesn't pace the loop).
		if time.Now().Sub(p.lastTP) >= TesterPresentPeriodMs*time.Millisecond {
			err := p.testerPresent(ctx)
			if err != nil {
				log.Printf("tester present failed: %s", err)
			}
			p.lastTP = time.Now()
		}
	}
}

// ===== Helpers (parity with Arduino) =====

func (p *SocketCAN) millis() uint32 {
	return uint32(time.Since(p.startTime) / time.Millisecond)
}

// ===== CAN I/O =====

func (p *SocketCAN) sendRaw(ctx context.Context, id uint32, data []byte) error {
	var frame can.Frame
	frame.ID = id
	frame.Length = uint8(len(data))
	copy(frame.Data[:], data)
	frame.IsExtended = false
	frame.IsRemote = false
	return p.tx.TransmitFrame(ctx, frame)
}

func k01GenerateKey(level int, seedHi, seedLo byte) (byte, byte) {
	var magic uint16
	if level == 3 {
		magic = 0x6F31
	} else {
		magic = 0x4D4E
	}
	s := (uint16(seedHi) << 8) | uint16(seedLo)
	k := uint16(uint32(magic*s) & 0xFFFF)
	return byte(k >> 8), byte(k & 0xFF)
}

func (p *SocketCAN) securityAccessLevel(level int) error {
	var reqSub, keySub byte
	if level == 3 {
		reqSub = SaL3RequestSeed
		keySub = SaL3SendKey
	} else {
		reqSub = SaL2RequestSeed
		keySub = SaL2SendKey
	}
	// request seed
	rsp, err := p.udsRequest([]byte{SidSecurityAccess, reqSub}, 32, 300*time.Millisecond)
	if err != nil || len(rsp) < 4 || rsp[0] != (SidSecurityAccess+PosOffset) || rsp[1] != reqSub {
		return errors.New("securityAccess: seed request failed")
	}
	seedHi, seedLo := rsp[2], rsp[3]
	kHi, kLo := k01GenerateKey(level, seedHi, seedLo)
	time.Sleep(100 * time.Millisecond)

	// send key (try a couple of times)
	for attempt := 0; attempt < 3; attempt++ {
		rsp2, err := p.udsRequest([]byte{SidSecurityAccess, keySub, kHi, kLo}, 16, 300*time.Millisecond)
		if err == nil && len(rsp2) >= 2 && rsp2[0] == (SidSecurityAccess+PosOffset) && rsp2[1] == keySub {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return errors.New("securityAccess: key rejected")
}

func (p *SocketCAN) testerPresent(ctx context.Context) error {
	// subfunction bit7=1 => suppress positive response
	return p.isotpSend(ctx, CanIdReq, []byte{SidTesterPresent, 0x80})
}

func (p *SocketCAN) readDID(did uint16) ([]byte, error) {
	req := []byte{SidReadDataByIdentifier, byte(did >> 8), byte(did & 0xFF)}
	rsp, err := p.udsRequest(req, 64, 300*time.Millisecond)
	if err != nil {
		return nil, err
	}
	if len(rsp) >= 3 &&
		rsp[0] == (SidReadDataByIdentifier+PosOffset) &&
		rsp[1] == byte(did>>8) && rsp[2] == byte(did&0xFF) {
		return rsp[3:], nil
	}
	return nil, errors.New("unexpected RDBI response")
}

// ===== CRC-8-CCITT (poly 0x07, init 0x00) =====

func crc8CCITTUpdate(crc, b byte) byte {
	crc ^= b
	for i := 0; i < 8; i++ {
		if (crc & 0x80) != 0 {
			crc = (crc << 1) ^ 0x07
		} else {
			crc <<= 1
		}
	}
	return crc
}

func crc8CCITTBuf(init byte, buf []byte) byte {
	crc := init
	for _, b := range buf {
		crc = crc8CCITTUpdate(crc, b)
	}
	return crc
}

// ===== Frame writer (exactly like Arduino's Serial.write stream) =====

func (p *SocketCAN) writeFrame(did uint16, data []byte) error {
	ms := p.millis()

	hdr := []byte{
		byte(ms),
		byte(ms >> 8),
		byte(ms >> 16),
		byte(ms >> 24),
		byte(did >> 8),
		byte(did),
		byte(len(data)),
	}

	crc := byte(0x00)
	crc = crc8CCITTBuf(crc, hdr[:4])   // millis LE
	crc = crc8CCITTUpdate(crc, hdr[4]) // DID hi
	crc = crc8CCITTUpdate(crc, hdr[5]) // DID lo
	crc = crc8CCITTUpdate(crc, hdr[6]) // len
	crc = crc8CCITTBuf(crc, data)

	// write: magic, header, payload, crc
	if _, err := p.writer.Write(magicBytes); err != nil {
		return err
	}
	if _, err := p.writer.Write(hdr); err != nil {
		return err
	}
	if _, err := p.writer.Write(data); err != nil {
		return err
	}
	if _, err := p.writer.Write([]byte{crc}); err != nil {
		return err
	}
	return nil
}
