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

	startAddress = 0x020000
	endAddress   = 0x140000 * 2
	chunkLength  = 0x80
)

const testerPresentInterval = 2 * time.Second

var lastTP time.Time

func main() {
	flags, _, _, socketCANFlags := config.GetFlags()
	if flags.Driver != config.SocketCAN {
		log.Fatalf("unsupported driver: %s", flags.Driver)
	}

	ifi, err := net.InterfaceByName(socketCANFlags.SocketCanAddr)
	if err != nil {
		log.Fatalf("lookup interface %s: %v", socketCANFlags.SocketCanAddr, err)
	}

	socketFile, _, err := openIsotpSocket(ifi.Index, canIDResponse, canIDRequest)
	if err != nil {
		log.Fatalf("open isotp: %v", err)
	}
	defer socketFile.Close()

	if err = doSecurityHandshake(socketFile); err != nil {
		log.Fatalf("security handshake failed: %v", err)
	}

	// Not necessary?
	/*err = doTesterPresent(socketFile)
	if err != nil {
		log.Fatalf("error on tester present: %v", err)
	}*/

	romFile, err := os.Create("rom.bin")
	if err != nil {
		log.Fatalf("create rom.bin: %v", err)
	}
	defer func(romFile *os.File) {
		err = romFile.Close()
		if err != nil {
			log.Fatalf("close rom.bin: %v", err)
		}
	}(romFile)

	var resp []byte
	for address := uint32(startAddress); address < endAddress; address += chunkLength {
		resp, err = sendAndReceiveBlocking(socketFile, buildReadMemoryRequest(address))
		if err != nil {
			log.Fatalf("error on read memory by address: %v", err)
		}
		_, err = romFile.Write(resp[1:])
		if err != nil {
			log.Fatalf("error on write rom chunk: %v", err)
		}

		fmt.Printf("%f\n", float64(address)/float64(endAddress-startAddress)*100)
	}
	// Write rom to disk
	err = romFile.Sync()
	if err != nil {
		log.Fatalf("error on write rom to disk: %v", err)
	}
}

func openIsotpSocket(interfaceIndex int, rxID, txID uint32) (*os.File, int, error) {
	fileDescriptor, err := unix.Socket(unix.AF_CAN, unix.SOCK_DGRAM, unix.CAN_ISOTP)
	if err != nil {
		return nil, -1, err
	}
	sa := &unix.SockaddrCAN{Ifindex: interfaceIndex, RxID: rxID, TxID: txID}
	if err = unix.Bind(fileDescriptor, sa); err != nil {
		err = unix.Close(fileDescriptor)
		if err != nil {
			return nil, -1, err
		}
		return nil, -1, err
	}
	file := os.NewFile(uintptr(fileDescriptor), "isotp")
	return file, fileDescriptor, nil
}

func buildReadMemoryRequest(address uint32) []byte {
	payload := make([]byte, 7)
	payload[0] = sidReadMemoryByAddress
	payload[1] = 0x00
	payload[2] = byte(address >> 16)
	payload[3] = byte(address >> 8)
	payload[4] = byte(address)
	payload[5] = byte(chunkLength)
	payload[6] = 0x00

	return payload
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

func doTesterPresent(conn *os.File) error {
	if time.Since(lastTP) >= testerPresentInterval {
		resp, err := sendAndReceiveBlocking(conn, []byte{0x3E, 0x00})
		if err != nil {
			return err
		}
		if resp[0] != 0x7E {
			return fmt.Errorf("error response from tester present % X", resp)
		}
		lastTP = time.Now()
	}
	return nil
}

func sendAndReceiveBlocking(conn *os.File, payload []byte) ([]byte, error) {
	// write (retry on EINTR)
	if err := writeBlocking(conn, payload); err != nil {
		return nil, err
	}
	// read (blocking; retry on EINTR).
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return buf[:n], nil
	}
}

func writeBlocking(conn *os.File, payload []byte) error {
	for {
		_, err := conn.Write(payload)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		return nil
	}
}
