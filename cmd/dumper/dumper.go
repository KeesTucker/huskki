package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"huskki/config"
	"huskki/ecus"

	"golang.org/x/sys/unix"
)

const (
	canIDRequest  = 0x7E0
	canIDResponse = 0x7E8

	sidSecurityAccess          = 0x27
	sidRequestUpload           = 0x35
	sidTransferData            = 0x36
	sidRequestTransferExit     = 0x37
	positiveResponseOffset     = 0x40
	securityAccessLevel2Seed   = 0x03
	securityAccessLevel2Key    = 0x04
	securityAccessLevel3Seed   = 0x05
	securityAccessLevel3Key    = 0x06
	addressAndLengthFormatByte = 0x31 // 3 address bytes, 1 size bytes
	dataFormatIdentifier       = 0x00 // no compression, no encryption

	maxChunkInitial = 0x20 // starting request size; will adapt down near the end
	minChunk        = 0x01 // do not go below 1
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

	networkInterface, err := net.InterfaceByName(socketCANFlags.SocketCanAddr)
	if err != nil {
		log.Fatalf("lookup interface %s: %v", socketCANFlags.SocketCanAddr, err)
	}

	socketDescriptor, err := unix.Socket(unix.AF_CAN, unix.SOCK_DGRAM, unix.CAN_ISOTP)
	if err != nil {
		log.Fatalf("create CAN_ISOTP socket: %v", err)
	}
	defer unix.Close(socketDescriptor)

	socketAddress := &unix.SockaddrCAN{
		Ifindex: networkInterface.Index,
		RxID:    canIDResponse,
		TxID:    canIDRequest,
	}
	if err := unix.Bind(socketDescriptor, socketAddress); err != nil {
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
		address         = 0x000000
		chunk           = maxChunkInitial
		waitRetryDelay  = 50 * time.Millisecond
		shrunkNearEnd   = false // becomes true when we first have to shrink due to out-of-range at a boundary
		lastGoodAddress = 0
	)

	for {
		data, nrc, err := requestUploadChunk(connection, address, chunk, waitRetryDelay)
		if err != nil {
			log.Fatalf("upload 0x%06X: %v", address, err)
		}

		if nrc != 0 {
			switch nrc {
			case nrcResponsePending:
				// ECU says slow down – quick backoff and retry same request
				time.Sleep(waitRetryDelay)
				continue

			case nrcRequestOutOfRange:
				// We tried to read past the end or across a boundary the ECU doesn't like.
				// Adaptively shrink the chunk and retry from the same address.
				previous := chunk
				chunk = shrinkChunk(chunk)
				if chunk < minChunk {
					chunk = minChunk
				}

				if previous != chunk {
					log.Printf("OOR at 0x%06X; shrinking chunk %d -> %d and retrying", address, previous, chunk)
				}

				// If we're already at min chunk and still OOR, we've reached the end.
				if chunk == minChunk {
					discovered := lastGoodAddress
					log.Printf("Reached end of ROM at 0x%06X (size: %d bytes / 0x%X).", discovered, discovered, discovered)
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

		if _, err := romFile.Write(data); err != nil {
			log.Fatalf("write rom.bin: %v", err)
		}
		lastGoodAddress = address + n
		address += n

		// If we had to shrink due to OOR previously, we may be at or near the end.
		if shrunkNearEnd {
			testData, nrc, err := requestUploadChunk(connection, address, minChunk, waitRetryDelay)
			if err != nil {
				log.Fatalf("probe read 0x%06X: %v", address, err)
			}
			if nrc == nrcRequestOutOfRange {
				// Confirmed end
				discovered := lastGoodAddress
				log.Printf("Discovered ROM size: %d bytes (0x%X).", discovered, discovered)
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
		_, _ = connection.Write([]byte{0x3E, 0x80})
		for {
			select {
			case <-ticker.C:
				_, _ = connection.Write([]byte{0x3E, 0x80})
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

func requestUploadChunk(connection *os.File, address int, size int, waitRetryDelay time.Duration) ([]byte, byte, error) {
	payload := []byte{
		sidRequestUpload,
		dataFormatIdentifier,
		addressAndLengthFormatByte,
		byte(address >> 16),
		byte(address >> 8),
		byte(address),
		byte(size),
	}

	resp, err := sendAndReceive(connection, payload)
	if err != nil {
		return nil, 0, err
	}
	if neg, nrc := parseNegative(resp, sidRequestUpload); neg {
		return nil, nrc, nil
	}
	if len(resp) < 3 || resp[0] != sidRequestUpload+positiveResponseOffset {
		return nil, 0, fmt.Errorf("unexpected upload response % X", resp)
	}
	// The upload response's block-length field (resp[1] & 0x0F bytes starting at resp[2])
	// indicates the maximum TransferData block size. We choose our own fixed size, so
	// this field is intentionally ignored.

	data := make([]byte, 0, size)
	blockCounter := byte(1)
	for len(data) < size {
		req := []byte{sidTransferData, blockCounter}
		resp, err := sendAndReceive(connection, req)
		if err != nil {
			return nil, 0, err
		}
		if neg, nrc := parseNegative(resp, sidTransferData); neg {
			if nrc == nrcResponsePending {
				time.Sleep(waitRetryDelay)
				continue
			}
			if nrc == nrcRequestOutOfRange {
				if err := requestTransferExit(connection, waitRetryDelay); err != nil {
					return nil, 0, err
				}
				return nil, nrc, nil
			}
			return nil, 0, fmt.Errorf("transfer data negative response NRC=0x%02X", nrc)
		}
		if len(resp) < 2 || resp[0] != sidTransferData+positiveResponseOffset || resp[1] != blockCounter {
			return nil, 0, fmt.Errorf("unexpected transfer data response % X", resp)
		}
		blockData := resp[2:]
		if len(blockData) > 0 {
			if len(data)+len(blockData) > size {
				blockData = blockData[:size-len(data)]
			}
			data = append(data, blockData...)
		}
		blockCounter++
	}

	if err := requestTransferExit(connection, waitRetryDelay); err != nil {
		return nil, 0, err
	}

	return data, 0, nil
}

func requestTransferExit(connection *os.File, waitRetryDelay time.Duration) error {
	for {
		resp, err := sendAndReceive(connection, []byte{sidRequestTransferExit})
		if err != nil {
			return err
		}
		if neg, nrc := parseNegative(resp, sidRequestTransferExit); neg {
			if nrc == nrcResponsePending {
				time.Sleep(waitRetryDelay)
				continue
			}
			return fmt.Errorf("transfer exit negative response NRC=0x%02X", nrc)
		}
		if len(resp) < 1 || resp[0] != sidRequestTransferExit+positiveResponseOffset {
			return fmt.Errorf("unexpected transfer exit response % X", resp)
		}
		break
	}
	return nil
}

func doSecurityHandshake(connection *os.File) error {
	// Level 3 security access: 27 03/04
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

	// Level 5 security access: 27 05/06
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

func sendAndReceive(connection *os.File, payload []byte) ([]byte, error) {
	if _, err := connection.Write(payload); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	n, err := connection.Read(buf)
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
