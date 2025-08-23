package drivers

import (
	"bufio"
	"context"
	"fmt"
	"huskki/config"
	"huskki/ecus"
	"huskki/store"
	"huskki/utils"
	"io"
	"log"
	"os"
	"sync"
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

	TesterPresentPeriod = 2 * time.Second

	MinDidGap            = 50 * time.Millisecond
	DefaultRespTimeout   = 300 * time.Millisecond
	FlushInterval        = 2 * time.Second
	SubscriberBufferSize = 4
)

var fastDIDs = []uint16{
	0x0100, 0x0009, 0x0076, 0x0070, 0x0001, 0x0031, 0x0110,
}

type SocketCAN struct {
	*config.SocketCANFlags
	ecuProcessor ecus.ECUProcessor

	conn   io.ReadWriteCloser
	recv   *socketcan.Receiver
	tx     *socketcan.Transmitter
	writer io.Writer

	startTime time.Time

	mu      sync.Mutex
	waiters map[uint32][]chan can.Frame
	ctx     context.Context
	cancel  context.CancelFunc

	lastChk  []byte
	lastLen  []byte
	lastRead []time.Time
}

func NewSocketCAN(flags *config.SocketCANFlags, ecuProcessor ecus.ECUProcessor) *SocketCAN {
	return &SocketCAN{
		SocketCANFlags: flags,
		ecuProcessor:   ecuProcessor,
		waiters:        make(map[uint32][]chan can.Frame),
	}
}

func (p *SocketCAN) Init() error {
	ctx := context.Background()
	conn, err := socketcan.DialContext(ctx, CanNetwork, p.SocketCanAddr)
	if err != nil {
		return fmt.Errorf("socketCAN connect %s: %w", p.SocketCanAddr, err)
	}
	p.conn = conn
	p.recv = socketcan.NewReceiver(conn)
	p.tx = socketcan.NewTransmitter(conn)

	// log file like Arduino
	filePath := utils.NextAvailableFilename(LOG_DIR, LOG_NAME, LOG_EXT)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open rawlog: %w", err)
	}
	p.writer = bufio.NewWriterSize(file, 1<<20)

	// per-DID state
	n := len(fastDIDs)
	p.lastChk = make([]byte, n)
	p.lastLen = make([]byte, n)
	p.lastRead = make([]time.Time, n)

	p.ctx, p.cancel = context.WithCancel(context.Background())
	p.startTime = time.Now()

	// start async reader first
	go p.recvLoop()
	// start tester-present ticker (non-blocking, no response expected)
	go p.testerPresentLoop()

	// raw-frame security handshake (single-frame)
	if err := p.DoSecurityHandshake(3); err != nil {
		return fmt.Errorf("security handshake failed: %w", err)
	}

	return nil
}

func (p *SocketCAN) Close() error {
	if p.cancel != nil {
		p.cancel()
	}
	if bw, ok := p.writer.(*bufio.Writer); ok {
		_ = bw.Flush()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

func (p *SocketCAN) Run() error {
	flushTicker := time.NewTicker(FlushInterval)
	defer flushTicker.Stop()

	idx := -1
	for {
		idx = (idx + 1) % len(fastDIDs)
		now := time.Now()

		if !p.lastRead[idx].IsZero() && now.Sub(p.lastRead[idx]) < MinDidGap {
			time.Sleep(5 * time.Millisecond)
			continue
		}

		did := fastDIDs[idx]
		req := []byte{SidReadDataByIdentifier, byte(did >> 8), byte(did)} // raw single-frame RDBI

		ctx, cancel := context.WithTimeout(p.ctx, DefaultRespTimeout)
		rsp, err := p.SendAndWait(ctx, CanIdReq, CanIdRsp, req)
		cancel()
		p.lastRead[idx] = now

		if err != nil {
			log.Printf("DID 0x%04X read error: %v", did, err)
		} else if len(rsp) >= 3 && rsp[0] == 0x62 && rsp[1] == byte(did>>8) && rsp[2] == byte(did) {
			data := rsp[3:]
			var chk byte
			for _, b := range data {
				chk ^= b
			}
			changed := (chk != p.lastChk[idx]) || (byte(len(data)) != p.lastLen[idx])
			if changed {
				if key, val := p.ecuProcessor.ParseDIDBytes(uint64(did), data); key != "" {
					if stream, ok := store.DashboardStreams[key]; ok {
						if stream.Discrete() {
							stream.Add(int(now.UnixMilli()), val)
						}
						stream.Add(int(now.UnixMilli()), val)
					}
				}
				_ = p.writeFrame(did, data)
				p.lastChk[idx] = chk
				p.lastLen[idx] = byte(len(data))
			}
		}

		select {
		case <-flushTicker.C:
			if bw, ok := p.writer.(*bufio.Writer); ok {
				_ = bw.Flush()
			}
		default:
		}
	}
}

// ---------------- Tester Present (background, non-blocking) ----------------

func (p *SocketCAN) testerPresentLoop() {
	t := time.NewTicker(TesterPresentPeriod)
	defer t.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-t.C:
			// 0x3E 0x80 : suppress positive response, so we don't wait for anything
			ctx, cancel := context.WithTimeout(p.ctx, 100*time.Millisecond)
			_ = p.sendRaw(ctx, CanIdReq, []byte{SidTesterPresent, 0x80})
			cancel()
		}
	}
}

// ---------------- RAW Security Handshake (single-frame) ----------------

func (p *SocketCAN) DoSecurityHandshake(level int) error {
	var reqSub, keySub byte
	switch level {
	case 3:
		reqSub, keySub = SaL3RequestSeed, SaL3SendKey
	default:
		reqSub, keySub = SaL2RequestSeed, SaL2SendKey
	}

	seedHi, seedLo, err := p.rawRequestSeed(reqSub)
	if err != nil {
		return err
	}
	kHi, kLo := k01GenerateKey(level, seedHi, seedLo)

	for attempt := 0; attempt < 3; attempt++ {
		ok, err := p.rawSendKey(keySub, kHi, kLo)
		if err == nil && ok {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("securityAccess: key rejected")
}

func (p *SocketCAN) rawRequestSeed(reqSub byte) (byte, byte, error) {
	ctx, cancel := context.WithTimeout(p.ctx, 300*time.Millisecond)
	defer cancel()
	rsp, err := p.SendAndWait(ctx, CanIdReq, CanIdRsp, []byte{SidSecurityAccess, reqSub})
	if err != nil {
		return 0, 0, err
	}
	if len(rsp) >= 4 && rsp[0] == (SidSecurityAccess+PosOffset) && rsp[1] == reqSub {
		return rsp[2], rsp[3], nil
	}
	if len(rsp) >= 3 && rsp[0] == 0x7F && rsp[1] == SidSecurityAccess {
		return 0, 0, fmt.Errorf("UDS NRC: 0x%02X", rsp[2])
	}
	return 0, 0, fmt.Errorf("unexpected seed response % X", rsp)
}

func (p *SocketCAN) rawSendKey(keySub, kHi, kLo byte) (bool, error) {
	ctx, cancel := context.WithTimeout(p.ctx, 300*time.Millisecond)
	defer cancel()
	rsp, err := p.SendAndWait(ctx, CanIdReq, CanIdRsp, []byte{SidSecurityAccess, keySub, kHi, kLo})
	if err != nil {
		return false, err
	}
	if len(rsp) >= 2 && rsp[0] == (SidSecurityAccess+PosOffset) && rsp[1] == keySub {
		return true, nil
	}
	if len(rsp) >= 3 && rsp[0] == 0x7F && rsp[1] == SidSecurityAccess {
		return false, fmt.Errorf("UDS NRC: 0x%02X", rsp[2])
	}
	return false, fmt.Errorf("unexpected key response % X", rsp)
}

// simple key algo (unchanged)
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

// ---------------- Async reader & router ----------------

func (p *SocketCAN) recvLoop() {
	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}
		if !p.recv.Receive() {
			_ = p.recv.Err()
			time.Sleep(time.Millisecond)
			continue
		}
		p.dispatch(p.recv.Frame())
	}
}

func (p *SocketCAN) dispatch(f can.Frame) {
	p.mu.Lock()
	defer p.mu.Unlock()
	list := p.waiters[f.ID]
	if len(list) == 0 {
		return
	}
	i := 0
	for _, ch := range list {
		select {
		case ch <- f:
		default:
			// never block; drop for this waiter if its buffer is full
		}
		list[i] = ch
		i++
	}
	p.waiters[f.ID] = list[:i]
}

func (p *SocketCAN) registerWaiter(id uint32, ch chan can.Frame) func() {
	p.mu.Lock()
	p.waiters[id] = append(p.waiters[id], ch)
	p.mu.Unlock()
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		list := p.waiters[id]
		for i, c := range list {
			if c == ch {
				list[i] = list[len(list)-1]
				list = list[:len(list)-1]
				break
			}
		}
		if len(list) == 0 {
			delete(p.waiters, id)
		} else {
			p.waiters[id] = list
		}
	}
}

// SendAndWait: send raw frame and wait for first frame with expectID (single-frame).
func (p *SocketCAN) SendAndWait(ctx context.Context, txID, expectID uint32, payload []byte) ([]byte, error) {
	if err := p.sendRaw(ctx, txID, payload); err != nil {
		return nil, err
	}
	ch := make(chan can.Frame, SubscriberBufferSize)
	unreg := p.registerWaiter(expectID, ch)
	defer unreg()

	select {
	case f := <-ch:
		return f.Data[:f.Length], nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ---------------- Raw send + writer ----------------

func (p *SocketCAN) sendRaw(ctx context.Context, id uint32, data []byte) error {
	var frame can.Frame
	frame.ID = id
	frame.Length = uint8(len(data))
	copy(frame.Data[:], data)
	frame.IsExtended = false
	frame.IsRemote = false
	return p.tx.TransmitFrame(ctx, frame)
}

func (p *SocketCAN) millis() uint32 {
	return uint32(time.Since(p.startTime) / time.Millisecond)
}

func (p *SocketCAN) writeFrame(did uint16, data []byte) error {
	ms := p.millis()
	hdr := []byte{
		byte(ms), byte(ms >> 8), byte(ms >> 16), byte(ms >> 24),
		byte(did >> 8), byte(did),
		byte(len(data)),
	}
	crc := byte(0x00)
	crc = crc8CCITTBuf(crc, hdr[:4])
	crc = crc8CCITTUpdate(crc, hdr[4])
	crc = crc8CCITTUpdate(crc, hdr[5])
	crc = crc8CCITTUpdate(crc, hdr[6])
	crc = crc8CCITTBuf(crc, data)

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

// ---------------- CRC-8-CCITT ----------------

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
