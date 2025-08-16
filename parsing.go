package main

import (
	"huskki/hub"
	"math"
)

const (
	COOLANT_OFFSET = -40
)

// Known DIDs
const (
	RPM_DID           = 0x0100
	THROTTLE_DID      = 0x0001
	GRIP_DID          = 0x0070
	TPS_DID           = 0x0076
	COOLANT_DID       = 0x0009
	GEAR              = 0x0031 // Gear enum
	INJECTION_TIME    = 0x0110 // Injection Time Cyl #1
	CLUTCH            = 0x0041 // Clutch
	O2_VOLTAGE        = 0x0012 // O2 Voltage Rear
	O2_COMPENSATION   = 0x0102 // O2 compensation #1
	IAP_VOLTAGE       = 0x0002 // IAP Cyl #1 Voltage
	IAP               = 0x0003 // IAP Cyl #1
	IGNITION_COIL_1   = 0x0120 // Ignition Cyl #1 Coil #1
	IGNITION_COIL_2   = 0x0108 // Ignition Cyl #1 Coil #2
	DWELL_TIME_COIL_1 = 0x0130 // Dwell time Cyl #1 Coil #1
	DWELL_TIME_COIL_2 = 0x0132 // Dwell time Cyl #1 Coil #2
	SWITCH1           = 0x0064 // Switch (second byte toggles)
	SWITCH2           = 0x0042 // Side stand?
)

func broadcastParsedSensorData(eventHub *hub.EventHub, didVal uint64, dataBytes []byte, timestamp int) {
	switch uint16(didVal) {
	case RPM_DID: // RPM = u16be / 4
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			rpm := raw / 4
			eventHub.Broadcast(map[string]any{"rpm": rpm, "timestamp": timestamp})
		}

	case THROTTLE_DID: // Throttle: (0..255) -> % (target ecu calculated throttle)
		if len(dataBytes) >= 1 {
			raw8 := int(dataBytes[len(dataBytes)-1])
			pct := roundTo1Dp(float64(raw8) / 255.0 * 100.0)
			eventHub.Broadcast(map[string]any{"throttle": pct, "timestamp": timestamp})
		}

	case GRIP_DID: // Grip: (0..255) -> % (gives raw pot value in percent from the throttle twist)
		if len(dataBytes) >= 1 {
			raw8 := int(dataBytes[len(dataBytes)-1])
			pct := roundTo1Dp(float64(raw8) / 255.0 * 100.0)
			eventHub.Broadcast(map[string]any{"grip": pct, "timestamp": timestamp})
		}

	case TPS_DID: // TPS (0..1023) -> % (throttle plate position sensor, idle is 20%, WOT is 100%)
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			pct := roundTo1Dp(float64(raw) / 1023.0 * 100.0)
			eventHub.Broadcast(map[string]any{"tps": pct, "timestamp": timestamp})
		}

	case COOLANT_DID: // Coolant °C
		if len(dataBytes) >= 2 {
			val := int(dataBytes[0])<<8 | int(dataBytes[1])
			eventHub.Broadcast(map[string]any{"coolant": val + COOLANT_OFFSET, "timestamp": timestamp})
		} else if len(dataBytes) == 1 {
			eventHub.Broadcast(map[string]any{"coolant": int(dataBytes[0]) + COOLANT_OFFSET, "timestamp": timestamp})
		}

	case GEAR:
		if len(dataBytes) >= 2 {
			val := int(dataBytes[1])
			eventHub.Broadcast(map[string]any{"gear": val, "timestamp": timestamp, "discrete": true})
		}

	case INJECTION_TIME:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			ms := roundTo2Dp(float64(raw) / 1000.0)
			eventHub.Broadcast(map[string]any{"injection time": ms, "timestamp": timestamp})
		}
	}
}

func roundTo1Dp(f float64) float64 {
	return math.Round(f*10) / 10
}

func roundTo2Dp(f float64) float64 {
	return math.Round(f*100) / 100
}
