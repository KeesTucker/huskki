package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
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

const testerPresentInterval = 2 * time.Second

func main() {
	flags, _, _, socketCANFlags := config.GetFlags()
	if flags.Driver != config.SocketCAN {
		log.Fatalf("unsupported driver: %s", flags.Driver)
	}

	ifi, err := net.InterfaceByName(socketCANFlags.SocketCanAddr)
	if err != nil {
		log.Fatalf("lookup interface %s: %v", socketCANFlags.SocketCanAddr, err)
	}

	conn, fd, err := openIsotpSocket(ifi.Index, canIDResponse, canIDRequest)
	if err != nil {
		log.Fatalf("open isotp: %v", err)
	}
	defer func() { _ = conn.Close(); _ = unix.Close(fd) }()

	if err := doSecurityHandshake(conn); err != nil {
		log.Fatalf("security handshake failed: %v", err)
	}

	romFile, err := os.Create("rom.bin")
	if err != nil {
		log.Fatalf("create rom.bin: %v", err)
	}
	defer romFile.Close()

	var (
		address                    = 0x000000
		chunk                      = maxChunkInitial
		waitRetryDelay             = 50 * time.Millisecond
		shrunkNearEnd              = false
		lastGoodAddress            = 0
		romStartLogged             = false
		successfulFormatIdentifier byte

		lastTP = time.Now()
	)

	for {
		// Safe TesterPresent between requests only
		if time.Since(lastTP) >= testerPresentInterval {
			_ = writeBlocking(conn, []byte{0x3E, 0x80}) // suppress positive response
			lastTP = time.Now()
		}

		data, nrc, err := readMemoryChunkBlocking(conn, address, chunk, &successfulFormatIdentifier)
		if err != nil {
			// Kernel-level timeouts / state errors:
			switch {
			case errors.Is(err, unix.ETIMEDOUT):
				// ISO-TP decided it timed out (e.g., no FC / no CF) – advance probe.
				address += chunk
				continue
			case errors.Is(err, unix.ECOMM):
				// Kernel says previous transfer still unwinding; re-open socket for clean slate.
				if err2 := reopenIsotpSocket(&conn, &fd, ifi.Index, canIDResponse, canIDRequest); err2 != nil {
					log.Fatalf("reopen isotp after ECOMM: %v (orig: %v)", err2, err)
				}
				// keep same address/chunk; try again
				time.Sleep(20 * time.Millisecond)
				continue
			default:
				log.Fatalf("read 0x%06X: %v", address, err)
			}
		}

		if nrc != 0 {
			switch nrc {
			case nrcResponsePending:
				time.Sleep(waitRetryDelay)
				continue
			case nrcRequestOutOfRange:
				prev := chunk
				chunk = shrinkChunk(prev)
				if chunk < minChunk {
					chunk = minChunk
				}
				if prev != chunk {
					log.Printf("OOR at 0x%06X; shrinking chunk %d -> %d and retrying", address, prev, chunk)
				}
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
			prev := chunk
			chunk = shrinkChunk(prev)
			if chunk < minChunk {
				chunk = minChunk
			}
			if prev != chunk {
				log.Printf("Empty data at 0x%06X; shrinking chunk %d -> %d and retrying", address, prev, chunk)
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

		if shrunkNearEnd {
			testData, nrc, err := readMemoryChunkBlocking(conn, address, minChunk, &successfulFormatIdentifier)
			if err != nil {
				if errors.Is(err, unix.ETIMEDOUT) {
					discovered := lastGoodAddress
					log.Printf("ROM end: 0x%06X", discovered-1)
					log.Printf("ROM size: %d bytes (0x%X)", discovered, discovered)
					writeSizeFile(discovered)
					log.Printf("ROM written to rom.bin")
					return
				}
				if errors.Is(err, unix.ECOMM) {
					if err2 := reopenIsotpSocket(&conn, &fd, ifi.Index, canIDResponse, canIDRequest); err2 != nil {
						log.Fatalf("reopen isotp after ECOMM: %v (orig: %v)", err2, err)
					}
					continue
				}
				log.Fatalf("probe read 0x%06X: %v", address, err)
			}
			if nrc == nrcRequestOutOfRange {
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
			_ = testData
		}
	}
}

func openIsotpSocket(ifindex int, rxID, txID uint32) (*os.File, int, error) {
	fd, err := unix.Socket(unix.AF_CAN, unix.SOCK_DGRAM, unix.CAN_ISOTP)
	if err != nil {
		return nil, -1, err
	}
	sa := &unix.SockaddrCAN{Ifindex: ifindex, RxID: rxID, TxID: txID}
	if err := unix.Bind(fd, sa); err != nil {
		unix.Close(fd)
		return nil, -1, err
	}
	f := os.NewFile(uintptr(fd), "isotp")
	return f, fd, nil
}

func reopenIsotpSocket(conn **os.File, fd *int, ifindex int, rxID, txID uint32) error {
	_ = (*conn).Close()
	_ = unix.Close(*fd)
	nc, nfd, err := openIsotpSocket(ifindex, rxID, txID)
	if err != nil {
		return err
	}
	*conn = nc
	*fd = nfd
	return nil
}

func shrinkChunk(current int) int {
	if current >= 0x20 {
		return current / 2
	}
	return current - 1
}

func parseNegative(resp []byte, requestSID byte) (bool, byte) {
	if len(resp) >= 3 && resp[0] == udsNegativeResponseSID && resp[1] == requestSID {
		return true, resp[2]
	}
	return false, 0
}

func readMemoryChunkBlocking(conn *os.File, address int, size int, chosenFormatIdentifier *byte) ([]byte, byte, error) {
	requiredAddressBytes := bytesNeededForAddress(address)

	// If we already picked an ALFID, ensure it still fits; otherwise re-probe.
	if *chosenFormatIdentifier != 0 {
		addrBytes := int((*chosenFormatIdentifier) >> 4)
		sizeBytes := int((*chosenFormatIdentifier) & 0x0F)
		if addrBytes < requiredAddressBytes || sizeExceeds(size, sizeBytes) {
			*chosenFormatIdentifier = 0
		}
	}

	// Reuse known-good ALFID.
	if *chosenFormatIdentifier != 0 {
		payload := buildReadMemoryRequest(address, size, *chosenFormatIdentifier)
		resp, err := sendAndReceiveBlocking(conn, payload)
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

	// Probe ALFIDs.
	for _, candidate := range commonAddressAndLengthFormatIdentifiers {
		addrBytes := int(candidate >> 4)
		sizeBytes := int(candidate & 0x0F)
		if addrBytes < requiredAddressBytes || sizeExceeds(size, sizeBytes) {
			continue
		}

		payload := buildReadMemoryRequest(address, size, candidate)
		resp, err := sendAndReceiveBlocking(conn, payload)
		if err != nil {
			// If kernel timed out or comm error on this candidate, try next candidate.
			if errors.Is(err, unix.ETIMEDOUT) || errors.Is(err, unix.ECOMM) {
				continue
			}
			return nil, 0, err
		}
		if neg, nrc := parseNegative(resp, sidReadMemoryByAddress); neg {
			if nrc == nrcRequestOutOfRange {
				continue
			}
			return nil, nrc, nil
		}
		if len(resp) < 1 || resp[0] != sidReadMemoryByAddress+positiveResponseOffset {
			return nil, 0, fmt.Errorf("unexpected read memory response % X", resp)
		}
		*chosenFormatIdentifier = candidate
		log.Printf("Using AddressAndLengthFormatIdentifier 0x%02X", candidate)
		return resp[1:], 0, nil
	}

	// Nothing worked; report timeout to let caller advance address.
	return nil, 0, unix.ETIMEDOUT
}

func buildReadMemoryRequest(address int, size int, formatIdentifier byte) []byte {
	addressBytes := int(formatIdentifier >> 4)
	sizeBytes := int(formatIdentifier & 0x0F)
	payload := make([]byte, 2+addressBytes+sizeBytes)
	payload[0] = sidReadMemoryByAddress
	payload[1] = formatIdentifier

	// big-endian address
	a := address
	for i := addressBytes - 1; i >= 0; i-- {
		payload[2+i] = byte(a)
		a >>= 8
	}
	// big-endian size
	offset := 2 + addressBytes
	s := size
	for i := sizeBytes - 1; i >= 0; i-- {
		payload[offset+i] = byte(s)
		s >>= 8
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
		return size > 0x7FFFFFFF
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

func doSecurityHandshake(conn *os.File) error {
	// 03/04
	resp, err := sendAndReceiveBlocking(conn, []byte{sidSecurityAccess, securityAccessLevel2Seed})
	if err != nil {
		return fmt.Errorf("request level 3 seed: %w", err)
	}
	if len(resp) < 4 || resp[0] != sidSecurityAccess+positiveResponseOffset || resp[1] != securityAccessLevel2Seed {
		return fmt.Errorf("unexpected level 3 seed response % X", resp)
	}
	seedHigh, seedLow := resp[2], resp[3]
	keyHigh, keyLow, err := ecus.GenerateK701Key(ecus.SecurityLevel2, seedHigh, seedLow)
	if err != nil {
		return fmt.Errorf("generate level 3 key: %w", err)
	}
	resp, err = sendAndReceiveBlocking(conn, []byte{sidSecurityAccess, securityAccessLevel2Key, keyHigh, keyLow})
	if err != nil {
		return fmt.Errorf("send level 3 key: %w", err)
	}
	if len(resp) < 2 || resp[0] != sidSecurityAccess+positiveResponseOffset || resp[1] != securityAccessLevel2Key {
		return fmt.Errorf("unexpected level 3 key response % X", resp)
	}
	log.Printf("Security access level 3 granted")

	// 05/06
	resp, err = sendAndReceiveBlocking(conn, []byte{sidSecurityAccess, securityAccessLevel3Seed})
	if err != nil {
		return fmt.Errorf("request level 5 seed: %w", err)
	}
	if len(resp) < 4 || resp[0] != sidSecurityAccess+positiveResponseOffset || resp[1] != securityAccessLevel3Seed {
		return fmt.Errorf("unexpected level 5 seed response % X", resp)
	}
	seedHigh, seedLow = resp[2], resp[3]
	keyHigh, keyLow, err = ecus.GenerateK701Key(ecus.SecurityLevel3, seedHigh, seedLow)
	if err != nil {
		return fmt.Errorf("generate level 5 key: %w", err)
	}
	resp, err = sendAndReceiveBlocking(conn, []byte{sidSecurityAccess, securityAccessLevel3Key, keyHigh, keyLow})
	if err != nil {
		return fmt.Errorf("send level 5 key: %w", err)
	}
	if len(resp) < 2 || resp[0] != sidSecurityAccess+positiveResponseOffset || resp[1] != securityAccessLevel3Key {
		return fmt.Errorf("unexpected level 5 key response % X", resp)
	}
	log.Printf("Security access level 5 granted")
	return nil
}

/*** BLOCKING, EINTR-SAFE I/O ***/

func sendAndReceiveBlocking(conn *os.File, payload []byte) ([]byte, error) {
	// write (retry on EINTR)
	if err := writeBlocking(conn, payload); err != nil {
		return nil, err
	}
	// read (blocking; retry on EINTR). Kernel ISO-TP controls the timeout.
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return nil, err // may be ETIMEDOUT, ECOMM, etc.
		}
		return buf[:n], nil
	}
}

func writeBlocking(conn *os.File, payload []byte) error {
	for {
		_, err := conn.Write(payload)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return err // propagate ETIMEDOUT/ECOMM/etc.
		}
		return nil
	}
}

/*** utils ***/

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
