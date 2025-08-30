package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"golang.org/x/sys/unix"
	"huskki/config"
	"huskki/ecus"
)

const (
	canIDRequest  = 0x7E0
	canIDResponse = 0x7E8

	sidSecurityAccess          = 0x27
	sidReadMemoryByAddress     = 0x23
	positiveResponseOffset     = 0x40
	securityAccessLevel3Seed   = 0x05
	securityAccessLevel3Key    = 0x06
	addressAndLengthFormatByte = 0x31 // 3 address bytes, 1 size byte

	maxChunkInitial = 0xF0 // starting request size; will adapt down near the end
	minChunk        = 0x01 // don't go below 1
)

// UDS / ISO-14229 negative response constants
const (
	udsNegativeResponseSID = 0x7F
	nrcRequestOutOfRange   = 0x31
	nrcResponsePending     = 0x78
)

func main() {
	flags, _, _, socketCANFlags := config.GetFlags()
	if flags.Driver != config.SocketCAN {
		log.Fatalf("unsupported driver: %s", flags.Driver)
	}

	iface, err := net.InterfaceByName(socketCANFlags.SocketCanAddr)
	if err != nil {
		log.Fatalf("lookup interface %s: %v", socketCANFlags.SocketCanAddr, err)
	}

	fd, err := unix.Socket(unix.AF_CAN, unix.SOCK_DGRAM, unix.CAN_ISOTP)
	if err != nil {
		log.Fatalf("create CAN_ISOTP socket: %v", err)
	}
	defer unix.Close(fd)

	addr := &unix.SockaddrCAN{
		Ifindex: iface.Index,
		RxID:    canIDResponse,
		TxID:    canIDRequest,
	}
	if err := unix.Bind(fd, addr); err != nil {
		log.Fatalf("bind CAN_ISOTP socket: %v", err)
	}

	conn := os.NewFile(uintptr(fd), "isotp")
	defer conn.Close()

	if err := doSecurityHandshake(conn); err != nil {
		log.Fatalf("security handshake failed: %v", err)
	}

	romFile, err := os.Create("rom.bin")
	if err != nil {
		log.Fatalf("create rom.bin: %v", err)
	}
	defer romFile.Close()

	var (
		address         = 0
		chunk           = maxChunkInitial
		totalWritten    = 0
		waitRetryDelay  = 50 * time.Millisecond
		ioTimeout       = 700 * time.Millisecond
		shrunkNearEnd   = false // becomes true when we first have to shrink due to out-of-range at a boundary
		lastGoodAddress = 0
	)

	for {
		// Build ReadMemoryByAddress payload for current address/chunk
		payload := []byte{
			sidReadMemoryByAddress,
			addressAndLengthFormatByte,
			byte(address >> 16),
			byte(address >> 8),
			byte(address),
			byte(chunk),
		}

		resp, err := sendAndReceive(conn, payload, ioTimeout)
		if err != nil {
			log.Fatalf("read 0x%06X: %v", address, err)
		}

		// Handle negative responses and ResponsePending
		isNeg, nrc := parseNegative(resp, sidReadMemoryByAddress)
		if isNeg {
			switch nrc {
			case nrcResponsePending:
				// ECU says slow down boi – quick backoff and retry same request
				time.Sleep(waitRetryDelay)
				continue

			case nrcRequestOutOfRange:
				// We tried to read past the end or across a boundary the ECU doesn't like.
				// Adaptively shrink the chunk and retry from the same address.
				prev := chunk
				chunk = shrinkChunk(chunk)
				if chunk < minChunk {
					chunk = minChunk
				}

				if prev != chunk {
					log.Printf("OOR at 0x%06X; shrinking chunk %d -> %d and retrying", address, prev, chunk)
				}

				// If we're already at min chunk and still OOR, we've reached the end.
				if chunk == minChunk {
					// If we never had a successful smaller read at this boundary, the true end is lastGoodAddress.
					discovered := lastGoodAddress
					log.Printf("Reached end of ROM at 0x%06X (size: %d bytes / 0x%X).", discovered, discovered, discovered)
					writeSizeFile(discovered)
					log.Printf("ROM written to rom.bin")
					return
				}

				shrunkNearEnd = true
				continue

			default:
				log.Fatalf("negative response (NRC=0x%02X) at 0x%06X: % X", nrc, address, resp)
			}
		}

		// Positive RMBA response starts with 0x63.
		if len(resp) < 1 || resp[0] != sidReadMemoryByAddress+positiveResponseOffset {
			log.Fatalf("unexpected response at 0x%06X: % X", address, resp)
		}

		// Standard layout: 0x63 0x31 <addr:3> <len:1> <data...>
		// data starts at index 6. len is echoed at resp[5].
		var data []byte
		var n int

		if len(resp) >= 6 && resp[1] == addressAndLengthFormatByte {
			dataStart := 6
			sizeEcho := int(resp[5])
			if sizeEcho < 0 {
				sizeEcho = 0
			}
			available := len(resp) - dataStart
			if available < 0 {
				available = 0
			}
			n = sizeEcho
			if n > available {
				n = available
			}
			// ECU might cap below requested chunk; we still accept what we got.
			if n > 0 {
				data = resp[dataStart : dataStart+n]
			}
		} else {
			// Fallback (non-standard ECU that omits header): treat resp[1:] as data, capped to chunk.
			data = resp[1:]
			if len(data) > chunk {
				data = data[:chunk]
			}
			n = len(data)
		}

		if n == 0 {
			// No data when positive? Treat as near-boundary anomaly: shrink and retry.
			prev := chunk
			chunk = shrinkChunk(chunk)
			if chunk < minChunk {
				chunk = minChunk
			}
			if prev != chunk {
				log.Printf("Empty data at 0x%06X; shrinking chunk %d -> %d and retrying", address, prev, chunk)
			}
			shrunkNearEnd = true
			continue
		}

		// Write and advance
		if _, err := romFile.Write(data); err != nil {
			log.Fatalf("write rom.bin: %v", err)
		}
		totalWritten += n
		lastGoodAddress = address + n
		address += n

		// If we had to shrink due to OOR previously, we may be at/near the end.
		if shrunkNearEnd {
			// Probe the next address with the smallest chunk to determine if we've hit the true end.
			testPayload := []byte{
				sidReadMemoryByAddress,
				addressAndLengthFormatByte,
				byte(address >> 16),
				byte(address >> 8),
				byte(address),
				byte(minChunk),
			}
			testResp, err := sendAndReceive(conn, testPayload, ioTimeout)
			if err != nil {
				log.Fatalf("probe read 0x%06X: %v", address, err)
			}
			if neg, nrc := parseNegative(testResp, sidReadMemoryByAddress); neg && nrc == nrcRequestOutOfRange {
				// Confirmed end
				discovered := lastGoodAddress
				log.Printf("Discovered ROM size: %d bytes (0x%X).", discovered, discovered)
				writeSizeFile(discovered)
				log.Printf("ROM written to rom.bin")
				return
			}
			// Not quite the end—continue reading from 'address'. Keep using current (possibly shrunk) chunk.
		}
	}
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

func doSecurityHandshake(connection *os.File) error {
	resp, err := sendAndReceive(connection, []byte{sidSecurityAccess, securityAccessLevel3Seed}, 300*time.Millisecond)
	if err != nil {
		return fmt.Errorf("request seed: %w", err)
	}
	if len(resp) < 4 || resp[0] != sidSecurityAccess+positiveResponseOffset || resp[1] != securityAccessLevel3Seed {
		return fmt.Errorf("unexpected seed response % X", resp)
	}
	seedHigh, seedLow := resp[2], resp[3]
	keyHigh, keyLow, err := ecus.GenerateK701Key(ecus.SecurityLevel3, seedHigh, seedLow)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	resp, err = sendAndReceive(connection, []byte{sidSecurityAccess, securityAccessLevel3Key, keyHigh, keyLow}, 300*time.Millisecond)
	if err != nil {
		return fmt.Errorf("send key: %w", err)
	}
	if len(resp) < 2 || resp[0] != sidSecurityAccess+positiveResponseOffset || resp[1] != securityAccessLevel3Key {
		return fmt.Errorf("unexpected key response % X", resp)
	}
	return nil
}

func sendAndReceive(conn *os.File, payload []byte, _ time.Duration) ([]byte, error) {
	// Write
	if _, err := conn.Write(payload); err != nil {
		return nil, err
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
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
