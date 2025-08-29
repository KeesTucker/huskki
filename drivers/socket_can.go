package drivers

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	"huskki/config"
	"huskki/ecus"
	"huskki/utils"
)

const (
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

	DefaultRespTimeout = 50 * time.Millisecond
	FlushInterval      = 2 * time.Second
)

type SocketCAN struct {
	*config.SocketCANFlags
	ecuProcessor ecus.ECUProcessor

	fd      int
	conn    *os.File
	writer  io.Writer
	logFile *os.File

	startTime time.Time

	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	waitChannel chan []byte

	lastChk  []byte
	lastLen  []byte
	lastRead []time.Time
}

func NewSocketCAN(flags *config.SocketCANFlags, ecuProcessor ecus.ECUProcessor) *SocketCAN {
	return &SocketCAN{
		SocketCANFlags: flags,
		ecuProcessor:   ecuProcessor,
	}
}

func (p *SocketCAN) Init() error {
	ifi, err := net.InterfaceByName(p.SocketCanAddr)
	if err != nil {
		return fmt.Errorf("lookup interface %s: %w", p.SocketCanAddr, err)
	}
	fd, err := unix.Socket(unix.AF_CAN, unix.SOCK_DGRAM, unix.CAN_ISOTP)
	if err != nil {
		return fmt.Errorf("socketCAN open: %w", err)
	}
	addr := &unix.SockaddrCAN{Ifindex: ifi.Index, RxID: CanIdRsp, TxID: CanIdReq}
	if err := unix.Bind(fd, addr); err != nil {
		unix.Close(fd)
		return fmt.Errorf("bind isotp: %w", err)
	}
	p.fd = fd
	p.conn = os.NewFile(uintptr(fd), fmt.Sprintf("isotp-%s", p.SocketCanAddr))

	// log file
	filePath := utils.NextAvailableFilename(LOG_DIR, LOG_NAME, LOG_EXT)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open rawlog: %w", err)
	}
	p.logFile = file
	p.writer = bufio.NewWriterSize(file, 1<<20)

	// per-DID state
	n := len(ecus.DIDsK701)
	p.lastChk = make([]byte, n)
	p.lastLen = make([]byte, n)
	p.lastRead = make([]time.Time, n)

	p.ctx, p.cancel = context.WithCancel(context.Background())
	p.startTime = time.Now()

	// start background receiver to drain unsolicited frames
	go p.receiveLoop()

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
	if p.logFile != nil {
		_ = p.logFile.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

func (p *SocketCAN) Run() error {
	flushTicker := time.NewTicker(FlushInterval)
	defer flushTicker.Stop()

	n := len(ecus.DIDsK701)
	startIdx := 0
	for {
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		default:
		}

		readyIdx := -1
		// lorg number
		minWait := time.Duration(1<<63 - 1)

		for i := 0; i < n; i++ {
			idx := (startIdx + i) % n
			did := ecus.DIDsK701[idx]

			if p.lastRead[idx].IsZero() {
				readyIdx = idx
				break
			}
			next := p.lastRead[idx].Add(ecus.DIDsToPollIntervalK701[did])
			wait := time.Until(next)
			if wait <= 0 {
				readyIdx = idx
				break
			}
			if wait < minWait {
				minWait = wait
			}
		}

		if readyIdx == -1 {
			timer := time.NewTimer(minWait)
			select {
			case <-p.ctx.Done():
				timer.Stop()
				return p.ctx.Err()
			case <-timer.C:
			}
			continue
		}

		did := ecus.DIDsK701[readyIdx]
		now := time.Now()

		req := []byte{SidReadDataByIdentifier, byte(did >> 8), byte(did)} // raw single-frame RDBI

		ctx, cancel := context.WithTimeout(p.ctx, DefaultRespTimeout)
		rsp, err := p.SendAndWait(ctx, CanIdReq, CanIdRsp, req)
		cancel()
		p.lastRead[readyIdx] = now

		if err != nil {
			log.Printf("DID 0x%04X read error: %v", did, err)
		} else if len(rsp) >= 3 && rsp[0] == 0x62 && rsp[1] == byte(did>>8) && rsp[2] == byte(did) {
			data := rsp[3:]
			var chk byte
			for _, b := range data {
				chk ^= b
			}
			changed := (chk != p.lastChk[readyIdx]) || (byte(len(data)) != p.lastLen[readyIdx])
			if changed {
				didData := p.ecuProcessor.ParseDIDBytes(did, data)
				addDidDataToStream(didData)
				err = p.writeFrameToBinary(did, data)
				if err != nil {
					log.Printf("writeFrameToBinary failed: %s", err)
				}
				p.lastChk[readyIdx] = chk
				p.lastLen[readyIdx] = byte(len(data))
			}
		}

		startIdx = (readyIdx + 1) % n

		select {
		case <-flushTicker.C:
			if bw, ok := p.writer.(*bufio.Writer); ok {
				_ = bw.Flush()
			}
		default:
		}
	}
}

func (p *SocketCAN) receiveLoop() {
	buffer := make([]byte, 4095)
	for {
		n, err := unix.Read(p.fd, buffer)
		if err != nil {
			if p.ctx.Err() != nil {
				return
			}
			continue
		}
		message := make([]byte, n)
		copy(message, buffer[:n])

		p.mu.Lock()
		responseChannel := p.waitChannel
		p.mu.Unlock()

		if responseChannel != nil {
			select {
			case responseChannel <- message:
			default:
			}
		}
	}
}

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

func (p *SocketCAN) DoSecurityHandshake(level ecus.SecurityLevel) error {
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
	keyHi, keyLo, err := ecus.GenerateK701Key(level, seedHi, seedLo)
	if err != nil {
		return err
	}

	for attempt := 0; attempt < 3; attempt++ {
		ok, err := p.rawSendKey(keySub, keyHi, keyLo)
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

func (p *SocketCAN) setDeadline(ctx context.Context) {
	var tv unix.Timeval
	if deadline, ok := ctx.Deadline(); ok {
		d := time.Until(deadline)
		if d < 0 {
			d = 0
		}
		tv = unix.NsecToTimeval(d.Nanoseconds())
	}
	_ = unix.SetsockoptTimeval(p.fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv)
	_ = unix.SetsockoptTimeval(p.fd, unix.SOL_SOCKET, unix.SO_SNDTIMEO, &tv)
}

// SendAndWait writes an ISO-TP payload and waits for a response.
func (p *SocketCAN) SendAndWait(ctx context.Context, txID, expectID uint32, payload []byte) ([]byte, error) {
	expectedServiceID := payload[0]
	positiveServiceID := expectedServiceID + PosOffset

	responseChannel := make(chan []byte, 4)

	p.mu.Lock()
	p.waitChannel = responseChannel
	p.setDeadline(ctx)
	if _, err := unix.Write(p.fd, payload); err != nil {
		p.waitChannel = nil
		p.mu.Unlock()
		return nil, err
	}
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.waitChannel = nil
		p.mu.Unlock()
	}()

	for {
		select {
		case message := <-responseChannel:
			if len(message) == 0 {
				continue
			}
			if message[0] == positiveServiceID {
				return append([]byte(nil), message...), nil
			}
			if message[0] == 0x7F && len(message) >= 3 && message[1] == expectedServiceID {
				if message[2] == 0x78 {
					continue
				}
				return append([]byte(nil), message...), nil
			}
			// ignore unsolicited frame
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (p *SocketCAN) sendRaw(ctx context.Context, id uint32, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.setDeadline(ctx)
	_, err := unix.Write(p.fd, payload)
	return err
}

func (p *SocketCAN) millis() uint32 {
	return uint32(time.Since(p.startTime) / time.Millisecond)
}
