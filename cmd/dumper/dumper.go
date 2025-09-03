package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"huskki/config"
	"huskki/ecus"

	"golang.org/x/sys/unix"
)

const (
	canIDRequest  = 0x7E0
	canIDResponse = 0x7E8

	sidSecurityAccess        = 0x27
	sidReadMemoryByAddress   = 0x23
	positiveResponseOffset   = 0x40
	securityAccessLevel2Seed = 0x03
	securityAccessLevel2Key  = 0x04
	securityAccessLevel3Seed = 0x05
	securityAccessLevel3Key  = 0x06

	maxChunkInitial = 0x20 // starting request size; will adapt down near the end
	minChunk        = 0x01 // do not go below 1
)

// commonAddressAndLengthFormatIdentifiers lists common AddressAndLengthFormatIdentifier
// combinations to probe when using ReadMemoryByAddress.
// 0x3x -> 3 address bytes, x length bytes
// 0x4x -> 4 address bytes, x length bytes
var commonAddressAndLengthFormatIdentifiers = []byte{
	0x31, 0x32, 0x33, 0x34,
	0x41, 0x42, 0x43, 0x44,
}

// UDS / ISO-14229 negative response constants
const (
	udsNegativeResponseSID = 0x7F
	nrcRequestOutOfRange   = 0x31
	nrcResponsePending     = 0x78
)

// ioBusy guards against sending TesterPresent during an in-flight request on the same ISO-TP socket.
var ioBusy int32

func main() {
	flags, _, _, socketCANFlags := config.GetFlags()
	if flags.Driver != config.SocketCAN {
		log.Fatalf("unsupported driver: %s", flags.Driver)
	}

	networkInterface, err := net.InterfaceByName(socketCANFlags.SocketCanAddr)
	if err != nil {
		log.Fatalf("lookup interface %s: %v", socketCANFlags.SocketCanAddr, err)
	}

	socketDescriptor, err := unix.Socket(unix.AF_CAN, unix.SOCK_DGRAM, unix.CAN_ISOTP)
	if err != nil {
		log.Fatalf("create CAN_ISOTP socket: %v", err)
	}

	socketAddress := &unix.SockaddrCAN{
		Ifindex: networkInterface.Index,
		RxID:    canIDResponse,
		TxID:    canIDRequest,
	}
	if err := unix.Bind(socketDescriptor, socketAddress); err != nil {
		unix.Close(socketDescriptor) // close on error path
		log.Fatalf("bind CAN_ISOTP socket: %v", err)
	}

	connection := os.NewFile(uintptr(socketDescriptor), "isotp")
	defer connection.Close()

	if err := doSecurityHandshake(connection); err != nil {
		log.Fatalf("security handshake failed: %v", err)
	}

	// Start periodic TesterPresent (3E 80 = suppress positive response)
	stopTesterPresent := startTesterPresent(connection, 2*time.Second)
	defer stopTesterPresent()

	romFile, err := os.Create("rom.bin")
	if err != nil {
		log.Fatalf("create rom.bin: %v", err)
	}
	defer romFile.Close()

	var (
		address                    = 0x000000
		chunk                      = maxChunkInitial
		waitRetryDelay             = 50 * time.Millisecond
		noResponseTimeout          = 100 * time.Millisecond
		shrunkNearEnd              = false // becomes true when we first have to shrink due to out-of-range at a boundary
		lastGoodAddress            = 0
		romStartLogged             = false
		successfulFormatIdentifier byte
	)

	for {
		data, nrc, err := readMemoryChunk(connection, address, chunk, noResponseTimeout, &successfulFormatIdentifier)
		if err != nil {
			// Intentionally probe forward on timeout to discover first readable address region.
			if errors.Is(err, os.ErrDeadlineExceeded) {
				address += chunk
				continue
			}
			log.Fatalf("read 0x%06X: %v", address, err)
		}

		if nrc != 0 {
			switch nrc {
			case nrcResponsePending:
				// ECU says slow down – quick backoff and retry same request
				time.Sleep(waitRetryDelay)
				continue

			case nrcRequestOutOfRange:
				// We tried to read past the end or across a boundary the ECU does not like.
				// Adaptively shrink the chunk and retry from the same address.
				previous := chunk
				chunk = shrinkChunk(chunk)
				if chunk < minChunk {
					chunk = minChunk
				}

				if previous != chunk {
					log.Printf("OOR at 0x%06X; shrinking chunk %d -> %d and retrying", address, previous, chunk)
				}

				// If we are already at min chunk and still OOR, we have reached the end.
				if chunk == minChunk {
					discovered := lastGoodAddress
					log.Printf("ROM end: 0x%06X", discovered-1)
					log.Printf("ROM size: %d bytes (0x%X)", discovered, discovered)
					writeSizeFile(discovered)
					log.Printf("ROM written to rom.bin")
					return
				}

				shrunkNearEnd = true
				continue

			default:
				log.Fatalf("negative response (NRC=0x%02X) at 0x%06X", nrc, address)
			}
		}

		n := len(data)
		if n == 0 {
			// No data when positive? Treat as near-boundary anomaly: shrink and retry.
			previous := chunk
			chunk = shrinkChunk(previous)
			if chunk < minChunk {
				chunk = minChunk
			}
			if previous != chunk {
				log.Printf("Empty data at 0x%06X; shrinking chunk %d -> %d and retrying", address, previous, chunk)
			}
			shrunkNearEnd = true
			continue
		}

		if !romStartLogged {
			log.Printf("ROM start: 0x%06X", address)
			romStartLogged = true
		}

		if _, err := romFile.Write(data); err != nil {
			log.Fatalf("write rom.bin: %v", err)
		}
		endAddress := address + n
		log.Printf("Read %d bytes from 0x%06X to 0x%06X", n, address, endAddress-1)
		lastGoodAddress = endAddress
		address = endAddress

		// If we had to shrink due to OOR previously, we may be at or near the end.
		if shrunkNearEnd {
			testData, nrc, err := readMemoryChunk(connection, address, minChunk, noResponseTimeout, &successfulFormatIdentifier)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					discovered := lastGoodAddress
					log.Printf("ROM end: 0x%06X", discovered-1)
					log.Printf("ROM size: %d bytes (0x%X)", discovered, discovered)
					writeSizeFile(discovered)
					log.Printf("ROM written to rom.bin")
					return
				}
				log.Fatalf("probe read 0x%06X: %v", address, err)
			}
			if nrc == nrcRequestOutOfRange {
				// Confirmed end
				discovered := lastGoodAddress
				log.Printf("ROM end: 0x%06X", discovered-1)
				log.Printf("ROM size: %d bytes (0x%X)", discovered, discovered)
				writeSizeFile(discovered)
				log.Printf("ROM written to rom.bin")
				return
			}
			if nrc == nrcResponsePending {
				time.Sleep(waitRetryDelay)
				continue
			}
			_ = testData // discard
			// Not quite the end—continue reading from 'address'. Keep using current (possibly shrunk) chunk.
		}
	}
}

func startTesterPresent(connection *os.File, interval time.Duration) (stop func()) {
	// Use sub-function 0x80 = suppress positive response to avoid extra reads.
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		// fire one immediately
		if atomic.LoadInt32(&ioBusy) == 0 {
			_, _ = connection.Write([]byte{0x3E, 0x80})
		}
		for {
			select {
			case <-ticker.C:
				// Skip if an ISO-TP exchange is in-flight.
				if atomic.LoadInt32(&ioBusy) == 0 {
					_, _ = connection.Write([]byte{0x3E, 0x80})
				}
			case <-done:
				return
			}
		}
	}()
	return func() { close(done) }
}

func shrinkChunk(current int) int {
	// First try to halve while it's reasonably large, then step down gently.
	if current >= 0x20 {
		return current / 2
	}
	// Small step down near the edge to find the exact boundary.
	return current - 1
}

func parseNegative(resp []byte, requestSID byte) (bool, byte) {
	// Negative response format: 0x7F, <requestSID>, <NRC>
	if len(resp) >= 3 && resp[0] == udsNegativeResponseSID && resp[1] == requestSID {
		return true, resp[2]
	}
	return false, 0
}

func readMemoryChunk(connection *os.File, address int, size int, timeout time.Duration, chosenFormatIdentifier *byte) ([]byte, byte, error) {
	requiredAddressBytes := bytesNeededForAddress(address)

	// If we already picked an ALFID earlier, make sure it still fits the growing address.
	if *chosenFormatIdentifier != 0 {
		addrBytes := int((*chosenFormatIdentifier) >> 4)
		sizeBytes := int((*chosenFormatIdentifier) & 0x0F)

		// If address no longer fits, drop the selection and re-probe.
		if addrBytes < requiredAddressBytes {
			*chosenFormatIdentifier = 0
		} else {
			// Sanity: ensure size still fits in the chosen size field (it does for our chunks, but be robust).
			if sizeExceeds(size, sizeBytes) {
				*chosenFormatIdentifier = 0
			}
		}
	}

	// Fast path: reuse previously successful ALFID when still valid.
	if *chosenFormatIdentifier != 0 {
		formatIdentifier := *chosenFormatIdentifier
		payload := buildReadMemoryRequest(address, size, formatIdentifier)
		resp, err := sendAndReceiveWithTimeout(connection, payload, timeout)
		if err != nil {
			return nil, 0, err
		}
		if neg, nrc := parseNegative(resp, sidReadMemoryByAddress); neg {
			return nil, nrc, nil
		}
		if len(resp) < 1 || resp[0] != sidReadMemoryByAddress+positiveResponseOffset {
			return nil, 0, fmt.Errorf("unexpected read memory response % X", resp)
		}
		return resp[1:], 0, nil
	}
	for _, candidate := range commonAddressAndLengthFormatIdentifiers {
		addressBytes := int(candidate >> 4)
		sizeBytes := int(candidate & 0x0F)

		if addressBytes < requiredAddressBytes {
			continue
		}
		if sizeExceeds(size, sizeBytes) {
			continue
		}

		payload := buildReadMemoryRequest(address, size, candidate)
		resp, err := sendAndReceiveWithTimeout(connection, payload, timeout)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				// No response for this candidate—try the next one.
				continue
			}
			return nil, 0, err
		}
		if neg, nrc := parseNegative(resp, sidReadMemoryByAddress); neg {
			if nrc == nrcRequestOutOfRange {
				// Try the next AddressAndLengthFormatIdentifier candidate
				continue
			}
			return nil, nrc, nil
		}
		if len(resp) < 1 || resp[0] != sidReadMemoryByAddress+positiveResponseOffset {
			return nil, 0, fmt.Errorf("unexpected read memory response % X", resp)
		}
		if *chosenFormatIdentifier == 0 {
			*chosenFormatIdentifier = candidate
			log.Printf("Using AddressAndLengthFormatIdentifier 0x%02X", candidate)
		}
		return resp[1:], 0, nil
	}

	// Nothing responded; let caller decide (your main loop advances address on timeouts).
	return nil, 0, os.ErrDeadlineExceeded
}

func buildReadMemoryRequest(address int, size int, formatIdentifier byte) []byte {
	addressBytes := int(formatIdentifier >> 4)
	sizeBytes := int(formatIdentifier & 0x0F)
	payload := make([]byte, 2+addressBytes+sizeBytes)
	payload[0] = sidReadMemoryByAddress
	payload[1] = formatIdentifier
	for i := addressBytes - 1; i >= 0; i-- {
		payload[2+i] = byte(address)
		address >>= 8
	}
	offset := 2 + addressBytes
	for i := sizeBytes - 1; i >= 0; i-- {
		payload[offset+i] = byte(size)
		size >>= 8
	}
	return payload
}

func sizeExceeds(size int, sizeBytes int) bool {
	switch sizeBytes {
	case 1:
		return size > 0xFF
	case 2:
		return size > 0xFFFF
	case 3:
		return size > 0xFFFFFF
	case 4:
		// int on 64-bit is plenty; practical upper bound is transport limits anyway
		return size > 0x7FFFFFFF // conservative
	default:
		return true
	}
}

func bytesNeededForAddress(address int) int {
	switch {
	case address <= 0xFF:
		return 1
	case address <= 0xFFFF:
		return 2
	case address <= 0xFFFFFF:
		return 3
	default:
		return 4
	}
}

func doSecurityHandshake(connection *os.File) error {
	// “Level 2” security (subfunctions 03/04)
	resp, err := sendAndReceive(connection, []byte{sidSecurityAccess, securityAccessLevel2Seed})
	if err != nil {
		err = fmt.Errorf("request level 3 seed: %w", err)
		log.Printf("Level 3 security access failed: %v", err)
		return err
	}
	if len(resp) < 4 || resp[0] != sidSecurityAccess+positiveResponseOffset || resp[1] != securityAccessLevel2Seed {
		err = fmt.Errorf("unexpected level 3 seed response % X", resp)
		log.Printf("Level 3 security access failed: %v", err)
		return err
	}
	seedHigh, seedLow := resp[2], resp[3]
	keyHigh, keyLow, err := ecus.GenerateK701Key(ecus.SecurityLevel2, seedHigh, seedLow)
	if err != nil {
		err = fmt.Errorf("generate level 3 key: %w", err)
		log.Printf("Level 3 security access failed: %v", err)
		return err
	}
	resp, err = sendAndReceive(connection, []byte{sidSecurityAccess, securityAccessLevel2Key, keyHigh, keyLow})
	if err != nil {
		err = fmt.Errorf("send level 3 key: %w", err)
		log.Printf("Level 3 security access failed: %v", err)
		return err
	}
	if len(resp) < 2 || resp[0] != sidSecurityAccess+positiveResponseOffset || resp[1] != securityAccessLevel2Key {
		err = fmt.Errorf("unexpected level 3 key response % X", resp)
		log.Printf("Level 3 security access failed: %v", err)
		return err
	}
	log.Printf("Security access level 3 granted")

	// “Level 3” security (subfunctions 05/06)
	resp, err = sendAndReceive(connection, []byte{sidSecurityAccess, securityAccessLevel3Seed})
	if err != nil {
		err = fmt.Errorf("request level 5 seed: %w", err)
		log.Printf("Level 5 security access failed: %v", err)
		return err
	}
	if len(resp) < 4 || resp[0] != sidSecurityAccess+positiveResponseOffset || resp[1] != securityAccessLevel3Seed {
		err = fmt.Errorf("unexpected level 5 seed response % X", resp)
		log.Printf("Level 5 security access failed: %v", err)
		return err
	}
	seedHigh, seedLow = resp[2], resp[3]
	keyHigh, keyLow, err = ecus.GenerateK701Key(ecus.SecurityLevel3, seedHigh, seedLow)
	if err != nil {
		err = fmt.Errorf("generate level 5 key: %w", err)
		log.Printf("Level 5 security access failed: %v", err)
		return err
	}
	resp, err = sendAndReceive(connection, []byte{sidSecurityAccess, securityAccessLevel3Key, keyHigh, keyLow})
	if err != nil {
		err = fmt.Errorf("send level 5 key: %w", err)
		log.Printf("Level 5 security access failed: %v", err)
		return err
	}
	if len(resp) < 2 || resp[0] != sidSecurityAccess+positiveResponseOffset || resp[1] != securityAccessLevel3Key {
		err = fmt.Errorf("unexpected level 5 key response % X", resp)
		log.Printf("Level 5 security access failed: %v", err)
		return err
	}
	log.Printf("Security access level 5 granted")
	return nil
}

func sendAndReceiveWithTimeout(connection *os.File, payload []byte, timeout time.Duration) ([]byte, error) {
	atomic.StoreInt32(&ioBusy, 1)
	defer atomic.StoreInt32(&ioBusy, 0)

	// Write with EINTR retry
	for {
		if _, err := connection.Write(payload); err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			return nil, err
		}
		break
	}

	fd := int(connection.Fd())
	pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN | unix.POLLERR | unix.POLLHUP}}
	deadline := time.Now().Add(timeout)

	// poll with EINTR retry and remaining-time budget
	for {
		ms := int(time.Until(deadline) / time.Millisecond)
		if ms <= 0 {
			return nil, os.ErrDeadlineExceeded
		}
		n, err := unix.Poll(pfd, ms)
		if errors.Is(err, unix.EINTR) {
			continue // retry
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, os.ErrDeadlineExceeded
		}
		// got an event; break to read
		break
	}

	buf := make([]byte, 4096)
	for {
		n, err := connection.Read(buf)
		if errors.Is(err, syscall.EINTR) {
			continue // retry read
		}
		if err != nil {
			return nil, err
		}
		return buf[:n], nil
	}
}

func sendAndReceive(connection *os.File, payload []byte) ([]byte, error) {
	atomic.StoreInt32(&ioBusy, 1)
	defer atomic.StoreInt32(&ioBusy, 0)

	// Write with EINTR retry
	for {
		if _, err := connection.Write(payload); err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			return nil, err
		}
		break
	}

	buf := make([]byte, 4096)
	for {
		n, err := connection.Read(buf)
		if errors.Is(err, syscall.EINTR) {
			continue // retry read
		}
		if err != nil {
			return nil, err
		}
		return buf[:n], nil
	}
}

func writeSizeFile(size int) {
	f, err := os.Create("rom.size")
	if err != nil {
		log.Printf("warn: could not create rom.size: %v", err)
		return
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "%d\n0x%X\n", size, size); err != nil {
		log.Printf("warn: writing rom.size: %v", err)
	}
}
