package ecus

import (
	"errors"
	"huskki/store"
	"huskki/utils"
	"maps"
	"slices"
	"time"
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

// DIDs
const (
	RpmDidK701                              = 0x0100
	ThrottleDidK701                         = 0x0001
	GripDidK701                             = 0x0070
	TpsDidK701                              = 0x0076
	CoolantDidK701                          = 0x0009
	GearDidK701                             = 0x0031
	InjectionTimeDidK701                    = 0x0110
	ClutchDidK701                           = 0x0041
	O2VoltageDidK701                        = 0x0012
	O2CompensationDidK701                   = 0x0102
	IapVoltageDidK701                       = 0x0002
	IapDidK701                              = 0x0003
	IgnitionCyl1Coil1DidK701                = 0x0120
	IgnitionCyl1Coil2DidK701                = 0x0108
	DwellTimeCyl1Coil1DidK701               = 0x0130
	DwellTimeCyl1Coil2DidK701               = 0x0132
	Switch1DidK701                          = 0x0064
	SideStandDidK701                        = 0x0042
	EngineLoadDidK701                       = 0x0007
	AtmosphericPressureDidK701              = 0x0004
	AtmosphericPressureSensorVoltageDidK701 = 0x0005
)

var DIDsToPollIntervalK701 = map[uint16]time.Duration{
	RpmDidK701:                              10 * time.Millisecond,
	ThrottleDidK701:                         10 * time.Millisecond,
	GripDidK701:                             10 * time.Millisecond,
	TpsDidK701:                              10 * time.Millisecond,
	CoolantDidK701:                          1 * time.Second,
	GearDidK701:                             10 * time.Millisecond,
	InjectionTimeDidK701:                    10 * time.Millisecond,
	ClutchDidK701:                           10 * time.Millisecond,
	O2VoltageDidK701:                        10 * time.Millisecond,
	O2CompensationDidK701:                   10 * time.Millisecond,
	IapVoltageDidK701:                       10 * time.Millisecond,
	IapDidK701:                              10 * time.Millisecond,
	IgnitionCyl1Coil1DidK701:                10 * time.Millisecond,
	IgnitionCyl1Coil2DidK701:                10 * time.Millisecond,
	DwellTimeCyl1Coil1DidK701:               10 * time.Millisecond,
	DwellTimeCyl1Coil2DidK701:               10 * time.Millisecond,
	Switch1DidK701:                          10 * time.Millisecond,
	SideStandDidK701:                        10 * time.Millisecond,
	EngineLoadDidK701:                       10 * time.Millisecond,
	AtmosphericPressureDidK701:              1 * time.Minute,
	AtmosphericPressureSensorVoltageDidK701: 1 * time.Minute,
}

var DIDsK701 = slices.Collect(maps.Keys(DIDsToPollIntervalK701))

// GenerateK701Key generates a 2 byte K701 key given a 2 byte seed and a level
func GenerateK701Key(level SecurityLevel, seedHi, seedLo byte) (keyHi, keyLo byte, err error) {
	var magicNumber uint16

	// Select magic number based on security level
	switch level {
	case SecurityLevel1:
		return 0x00, 0x00, errors.New("missing magic number for Level 1")
	case SecurityLevel2:
		magicNumber = 0x4D4E
	case SecurityLevel3:
		magicNumber = 0x6F31
	default:
		return 0x00, 0x00, errors.New("invalid level security level requested")
	}

	// Combine seed bytes into a single 16-bit value
	x := (uint16(seedHi) << 8) | uint16(seedLo)

	// Calculate the key
	key := (magicNumber * x) & 0xFFFF

	keyHi = byte((key >> 8) & 0xFF)
	keyLo = byte(key & 0xFF)

	return keyHi, keyLo, nil
}

func (k *K701) ParseDIDBytes(did uint64, dataBytes []byte) (key string, value float64) {
	switch uint16(did) {
	case RpmDidK701: // RPM = u16be / 4
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			rpm := float64(raw) / 4.0
			return store.RPM_STREAM, rpm
		}

	case ThrottleDidK701: // Throttle: (0..255) -> % (target ecu calculated throttle)
		if len(dataBytes) >= 1 {
			raw8 := int(dataBytes[len(dataBytes)-1])
			throttle := utils.RoundTo1Dp(float64(raw8) / 255.0 * 100.0)
			return store.THROTTLE_STREAM, throttle
		}

	case GripDidK701: // Grip: (0..255) -> % (gives raw pot value in percent from the throttle twist)
		if len(dataBytes) >= 1 {
			raw8 := int(dataBytes[len(dataBytes)-1])
			grip := utils.RoundTo1Dp(float64(raw8) / 255.0 * 100.0)
			return store.GRIP_STREAM, grip
		}

	case TpsDidK701: // TPS (0..1023) -> % (throttle plate position sensor, idle is 20%, WOT is 100%)
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			tps := utils.RoundTo1Dp(float64(raw) / 1023.0 * 100.0)
			return store.TPS_STREAM, tps
		}

	case CoolantDidK701: // Coolant °C
		temp := coolantOffset
		if len(dataBytes) >= 2 {
			temp += float64(int(dataBytes[0])<<8 | int(dataBytes[1]))

		} else if len(dataBytes) == 1 {
			temp += float64(int(dataBytes[0]))
		}
		return store.COOLANT_STREAM, temp

	case GearDidK701:
		if len(dataBytes) >= 2 {
			gear := float64(int(dataBytes[1]))
			return store.GEAR_STREAM, gear
		}

	case InjectionTimeDidK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			time := utils.RoundTo2Dp(float64(raw) / 1000.0)
			return store.INJECTION_TIME_STREAM, time
		}
	}
	return
}
