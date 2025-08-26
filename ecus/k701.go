package ecus

import (
	"errors"
	"huskki/store"
	"huskki/utils"
	"maps"
	"math"
	"slices"
	"time"
)

type SecurityLevel int8

const (
	_ SecurityLevel = iota
	SecurityLevel1
	SecurityLevel2
	SecurityLevel3
)

type K701 struct{}

const (
	coolantOffset                 = -40.0
	q151x                         = 32768.0
	mmHgTohPa                     = 1.33322
	hPaAtSeaLevel                 = 1013.25
	hPaHeightCoefficient          = 44330
	pressureAltitudeRatioExponent = 0.1903
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
	LeversDidK701                           = 0x0030
	O2Cyl1VoltageDidK701                    = 0x0012
	O2Cyl1CompensationDidK701               = 0x0102
	O2Cyl1AdcDidK701                        = 0x1009
	O2Cyl1ExtendedK701                      = 0xE5002
	IAPVoltageDidK701                       = 0x0002
	IapDidK701                              = 0x0003
	IgnitionCyl1Coil1DidK701                = 0x0120
	IgnitionCyl1Coil2DidK701                = 0x0108
	DwellTimeCyl1Coil1DidK701               = 0x0130
	DwellTimeCyl1Coil2DidK701               = 0x0132
	SASValveDidK701                         = 0x0064
	SideStandDidK701                        = 0x0042
	EngineLoadDidK701                       = 0x0007
	AtmosphericPressureDidK701              = 0x0004
	AtmosphericPressureSensorVoltageDidK701 = 0x0005
	Unknown1DidK701                         = 0x0041
)

var DIDsToPollIntervalK701 = map[uint32]time.Duration{
	RpmDidK701:                              10 * time.Millisecond,
	ThrottleDidK701:                         10 * time.Millisecond,
	GripDidK701:                             10 * time.Millisecond,
	TpsDidK701:                              10 * time.Millisecond,
	CoolantDidK701:                          1 * time.Second,
	GearDidK701:                             10 * time.Millisecond,
	InjectionTimeDidK701:                    10 * time.Millisecond,
	LeversDidK701:                           10 * time.Millisecond,
	O2Cyl1VoltageDidK701:                    10 * time.Millisecond,
	O2Cyl1CompensationDidK701:               10 * time.Millisecond,
	IAPVoltageDidK701:                       10 * time.Millisecond,
	O2Cyl1ExtendedK701:                      10 * time.Millisecond,
	O2Cyl1AdcDidK701:                        10 * time.Millisecond,
	IapDidK701:                              10 * time.Millisecond,
	IgnitionCyl1Coil1DidK701:                10 * time.Millisecond,
	IgnitionCyl1Coil2DidK701:                10 * time.Millisecond,
	DwellTimeCyl1Coil1DidK701:               10 * time.Millisecond,
	DwellTimeCyl1Coil2DidK701:               10 * time.Millisecond,
	SASValveDidK701:                         200 * time.Millisecond,
	SideStandDidK701:                        1 * time.Second,
	EngineLoadDidK701:                       10 * time.Millisecond,
	AtmosphericPressureDidK701:              10 * time.Second,
	AtmosphericPressureSensorVoltageDidK701: 10 * time.Second,
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

func (k *K701) ParseDIDBytes(did uint32, dataBytes []byte) []*DIDData {
	switch did {
	case RpmDidK701: // RPM = u16be / 4
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			rpm := float64(raw) / 4.0
			return []*DIDData{{store.RPM_STREAM, rpm}}
		}

	case ThrottleDidK701: // Throttle: (0..255) -> % (target ecu calculated throttle)
		if len(dataBytes) >= 1 {
			raw8 := int(dataBytes[len(dataBytes)-1])
			throttle := utils.RoundToXDp(float64(raw8)/255.0*100.0, 1)
			return []*DIDData{{store.THROTTLE_STREAM, throttle}}
		}

	case GripDidK701: // Grip: (0..255) -> % (gives raw pot value in percent from the throttle twist)
		if len(dataBytes) >= 1 {
			raw8 := int(dataBytes[len(dataBytes)-1])
			grip := utils.RoundToXDp(float64(raw8)/255.0*100.0, 1)
			return []*DIDData{{store.GRIP_STREAM, grip}}
		}

	case TpsDidK701: // TPS (0..1023) -> % (throttle plate position sensor, idle is 20%, WOT is 100%)
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			tps := utils.RoundToXDp(float64(raw)/1023.0*100.0, 1)
			return []*DIDData{{store.TPS_STREAM, tps}}
		}

	case CoolantDidK701: // Coolant °C
		temp := coolantOffset
		if len(dataBytes) >= 2 {
			temp += float64(int(dataBytes[0])<<8 | int(dataBytes[1]))

		} else if len(dataBytes) == 1 {
			temp += float64(int(dataBytes[0]))
		}
		return []*DIDData{{store.COOLANT_STREAM, temp}}

	case GearDidK701:
		if len(dataBytes) >= 2 {
			gear := float64(int(dataBytes[1]))
			return []*DIDData{{store.GEAR_STREAM, gear}}
		}

	case InjectionTimeDidK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			ms := utils.RoundToXDp(float64(raw)/1000.0, 2)
			return []*DIDData{{store.INJECTION_TIME_STREAM, ms}}
		}

	case SideStandDidK701:
		if len(dataBytes) >= 2 {
			down := dataBytes[1] == 0xFF
			return []*DIDData{{store.SIDESTAND_STREAM, utils.BoolToFloat(down)}}
		}

	case SASValveDidK701:
		if len(dataBytes) >= 2 {
			open := dataBytes[1] == 0xFF
			return []*DIDData{{store.SAS_VALVE_STREAM, utils.BoolToFloat(open)}}
		}

	case O2Cyl1VoltageDidK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			v := utils.RoundToXDp(float64(raw)/1023.0*5, 2)
			return []*DIDData{{store.CYL1_O2_VOLT_STREAM, v}}
		}

	case O2Cyl1CompensationDidK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			correction := utils.RoundToXDp(float64(raw)/q151x-1.0, 2)
			return []*DIDData{{store.CYL1_O2_COMP_STREAM, correction}}
		}

	case O2Cyl1AdcDidK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			return []*DIDData{{store.CYL1_O2_ADC_STREAM, float64(raw)}}
		}

	case O2Cyl1ExtendedK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			v := utils.RoundToXDp(float64(raw)/500.0, 2)
			return []*DIDData{{store.CYL1_O2_EXTENDED_STREAM, v}}
		}

	case IAPVoltageDidK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			return []*DIDData{{store.IAP_VOLTAGE_STREAM, float64(raw)}}
		}

	case IapDidK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			return []*DIDData{{store.IAP_STREAM, float64(raw)}}
		}

	case IgnitionCyl1Coil1DidK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			a := utils.RoundToXDp(float64(raw)/10.0, 1)
			return []*DIDData{{store.CYL1_COIL1_STREAM, a}}
		}

	case IgnitionCyl1Coil2DidK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			a := utils.RoundToXDp(float64(raw)/10.0, 1)
			return []*DIDData{{store.CYL1_COIL2_STREAM, a}}
		}

	case DwellTimeCyl1Coil1DidK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			ms := utils.RoundToXDp(float64(raw)/1000.0, 2)
			return []*DIDData{{store.CYL1_COIL1_DWELL_STREAM, ms}}
		}

	case DwellTimeCyl1Coil2DidK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			ms := utils.RoundToXDp(float64(raw)/1000.0, 2)
			return []*DIDData{{store.CYL1_COIL2_DWELL_STREAM, ms}}
		}

	case EngineLoadDidK701:
		if len(dataBytes) >= 1 {
			raw8 := int(dataBytes[len(dataBytes)-1])
			pct := utils.RoundToXDp(float64(raw8)/255.0*100.0, 1)
			return []*DIDData{{store.ENGINE_LOAD_STREAM, pct}}
		}

	case AtmosphericPressureSensorVoltageDidK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			v := utils.RoundToXDp(float64(raw)/10000.0, 3)
			return []*DIDData{{store.BARO_VOLT_STREAM, v}}
		}

	case AtmosphericPressureDidK701:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			hPa := float64(raw) * mmHgTohPa
			m := hPaHeightCoefficient * (1.0 - math.Pow(hPa/hPaAtSeaLevel, pressureAltitudeRatioExponent))
			m = utils.RoundToXDp(m, 1)
			return []*DIDData{{store.BARO_STREAM, m}}
		}

	case LeversDidK701:
		if len(dataBytes) >= 2 {
			clutchOut := dataBytes[0] == 0xFF
			frontBrake := utils.RoundToXDp(float64(int(dataBytes[1]))/255.0*100, 1)
			return []*DIDData{
				{
					store.CLUTCH_STREAM,
					utils.BoolToFloat(clutchOut),
				},
				{
					store.FRONT_BRAKE_STREAM,
					frontBrake,
				},
			}
		}
	}

	return []*DIDData{}
}
