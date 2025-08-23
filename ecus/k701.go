package ecus

import (
	"errors"
	"huskki/config"
	"huskki/utils"
)

type SecurityLevel int8

const (
	SecurityLevelUnspecified SecurityLevel = iota
	SecurityLevel1
	SecurityLevel2
	SecurityLevel3
)

type K701 struct{}

const (
	coolantOffset = -40.0
)

// Known DIDs
const (
	rpmDID            = 0x0100
	throttleDID       = 0x0001
	gripDID           = 0x0070
	tpsDid            = 0x0076
	coolantDid        = 0x0009
	gearDid           = 0x0031 // Gear enum
	injectionTimeDid  = 0x0110 // Injection Time Cyl #1
	clutchDid         = 0x0041 // Clutch
	o2VoltageDid      = 0x0012 // O2 Voltage Rear
	o2CompensationDid = 0x0102 // O2 compensation #1
	iapVoltageDid     = 0x0002 // IAP Cyl #1 Voltage
	iapDid            = 0x0003 // IAP Cyl #1
	ignitionCoil1Did  = 0x0120 // Ignition Cyl #1 Coil #1
	ignitionCoil2Did  = 0x0108 // Ignition Cyl #1 Coil #2
	dwellTimeCoil1Did = 0x0130 // Dwell time Cyl #1 Coil #1
	dwellTimeCoil2Did = 0x0132 // Dwell time Cyl #1 Coil #2
	switch1Did        = 0x0064 // Switch (second byte toggles)
	switch2Did        = 0x0042 // Side stand?
)

// GenerateK701Key generates a 2 byte K701 key given a 2 byte seed and a level
func GenerateK701Key(seed [2]byte, level SecurityLevel) ([2]byte, error) {
	var magicNumber uint16

	// Select magic number based on security level
	switch level {
	case SecurityLevel1:
		return [2]byte{}, errors.New("missing magic number for Level 1")
	case SecurityLevel2:
		magicNumber = 0x4D4E
	case SecurityLevel3:
		magicNumber = 0x6F31
	default:
		return [2]byte{}, errors.New("invalid level in generateSeedKey")
	}

	// Combine seed bytes into a single 16-bit value
	x := (uint16(seed[0]) << 8) | uint16(seed[1])

	// Calculate the key
	key := (magicNumber * x) & 0xFFFF

	// Split key into two bytes
	keyBytes := [2]byte{
		byte((key >> 8) & 0xFF),
		byte(key & 0xFF),
	}

	return keyBytes, nil
}

func (k *K701) ParseDIDBytes(did uint64, dataBytes []byte) (key string, value float64) {
	switch uint16(did) {
	case rpmDID: // RPM = u16be / 4
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			rpm := float64(raw) / 4.0
			return config.RPM_STREAM, rpm
		}

	case throttleDID: // Throttle: (0..255) -> % (target ecu calculated throttle)
		if len(dataBytes) >= 1 {
			raw8 := int(dataBytes[len(dataBytes)-1])
			throttle := utils.RoundTo1Dp(float64(raw8) / 255.0 * 100.0)
			return config.THROTTLE_STREAM, throttle
		}

	case gripDID: // Grip: (0..255) -> % (gives raw pot value in percent from the throttle twist)
		if len(dataBytes) >= 1 {
			raw8 := int(dataBytes[len(dataBytes)-1])
			grip := utils.RoundTo1Dp(float64(raw8) / 255.0 * 100.0)
			return config.GRIP_STREAM, grip
		}

	case tpsDid: // TPS (0..1023) -> % (throttle plate position sensor, idle is 20%, WOT is 100%)
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			tps := utils.RoundTo1Dp(float64(raw) / 1023.0 * 100.0)
			return config.TPS_STREAM, tps
		}

	case coolantDid: // Coolant °C
		temp := coolantOffset
		if len(dataBytes) >= 2 {
			temp += float64(int(dataBytes[0])<<8 | int(dataBytes[1]))

		} else if len(dataBytes) == 1 {
			temp += float64(int(dataBytes[0]))
		}
		return config.COOLANT_STREAM, temp

	case gearDid:
		if len(dataBytes) >= 2 {
			gear := float64(int(dataBytes[1]))
			return config.GEAR_STREAM, gear
		}

	case injectionTimeDid:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			time := utils.RoundTo2Dp(float64(raw) / 1000.0)
			return config.INJECTION_TIME_STREAM, time
		}
	}
	return
}
