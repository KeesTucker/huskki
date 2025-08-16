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
	RPM_DID               = 0x0100
	THROTTLE_DID          = 0x0001
	GRIP_DID              = 0x0070
	TPS_DID               = 0x0076
	COOLANT_DID           = 0x0009
	GEAR_DID              = 0x0031 // Gear enum
	INJECTION_TIME_DID    = 0x0110 // Injection Time Cyl #1
	CLUTCH_DID            = 0x0041 // Clutch
	O2_VOLTAGE_DID        = 0x0012 // O2 Voltage Rear
	O2_COMPENSATION_DID   = 0x0102 // O2 compensation #1
	IAP_VOLTAGE_DID       = 0x0002 // IAP Cyl #1 Voltage
	IAP_DID               = 0x0003 // IAP Cyl #1
	IGNITION_COIL_1_DID   = 0x0120 // Ignition Cyl #1 Coil #1
	IGNITION_COIL_2_DID   = 0x0108 // Ignition Cyl #1 Coil #2
	DWELL_TIME_COIL_1_DID = 0x0130 // Dwell time Cyl #1 Coil #1
	DWELL_TIME_COIL_2_DID = 0x0132 // Dwell time Cyl #1 Coil #2
	SWITCH_1_DID          = 0x0064 // Switch (second byte toggles)
	SWITCH_2_DID          = 0x0042 // Side stand?
)

func broadcastParsedSensorData(eventHub *hub.EventHub, didVal uint64, dataBytes []byte, timestamp int) {
	switch uint16(didVal) {
	case RPM_DID: // RPM = u16be / 4
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			rpm := raw / 4
			eventHub.Broadcast(&hub.Event{RPM_STREAM, timestamp, rpm})
		}

	case THROTTLE_DID: // Throttle: (0..255) -> % (target ecu calculated throttle)
		if len(dataBytes) >= 1 {
			raw8 := int(dataBytes[len(dataBytes)-1])
			percent := roundTo1Dp(float64(raw8) / 255.0 * 100.0)
			eventHub.Broadcast(&hub.Event{THROTTLE_STREAM, timestamp, percent})
		}

	case GRIP_DID: // Grip: (0..255) -> % (gives raw pot value in percent from the throttle twist)
		if len(dataBytes) >= 1 {
			raw8 := int(dataBytes[len(dataBytes)-1])
			percent := roundTo1Dp(float64(raw8) / 255.0 * 100.0)
			eventHub.Broadcast(&hub.Event{GRIP_STREAM, timestamp, percent})
		}

	case TPS_DID: // TPS (0..1023) -> % (throttle plate position sensor, idle is 20%, WOT is 100%)
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			pct := roundTo1Dp(float64(raw) / 1023.0 * 100.0)
			eventHub.Broadcast(&hub.Event{TPS_STREAM, timestamp, pct})
		}

	case COOLANT_DID: // Coolant °C
		if len(dataBytes) >= 2 {
			val := int(dataBytes[0])<<8 | int(dataBytes[1])
			eventHub.Broadcast(&hub.Event{COOLANT_STREAM, timestamp, val + COOLANT_OFFSET})
		} else if len(dataBytes) == 1 {
			eventHub.Broadcast(&hub.Event{COOLANT_STREAM, timestamp, int(dataBytes[0]) + COOLANT_OFFSET})
		}

	case GEAR_DID:
		if len(dataBytes) >= 2 {
			val := int(dataBytes[1])
			eventHub.Broadcast(&hub.Event{GEAR_STREAM, timestamp, val})
		}

	case INJECTION_TIME_DID:
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			ms := roundTo2Dp(float64(raw) / 1000.0)
			eventHub.Broadcast(&hub.Event{INJECTION_TIME_STREAM, timestamp, ms})
		}
	}
}

func roundTo1Dp(f float64) float64 {
	return math.Round(f*10) / 10
}

func roundTo2Dp(f float64) float64 {
	return math.Round(f*100) / 100
}
