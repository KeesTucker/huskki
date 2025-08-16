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
	RPM_DID      = 0x0100
	THROTTLE_DID = 0x0001
	GRIP_DID     = 0x0070
	TPS_DID      = 0x0076
	COOLANT_DID  = 0x0009
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
			pct := int(math.Round(float64(raw8) / 255.0 * 100.0))
			eventHub.Broadcast(map[string]any{"throttle": pct, "timestamp": timestamp})
		}

	case GRIP_DID: // Grip: (0..255) -> % (gives raw pot value in percent from the throttle twist)
		if len(dataBytes) >= 1 {
			raw8 := int(dataBytes[len(dataBytes)-1])
			pct := int(math.Round(float64(raw8) / 255.0 * 100.0))
			eventHub.Broadcast(map[string]any{"grip": pct, "timestamp": timestamp})
		}

	case TPS_DID: // TPS (0..1023) -> % (throttle plate position sensor, idle is 20%, WOT is 100%)
		if len(dataBytes) >= 2 {
			raw := int(dataBytes[0])<<8 | int(dataBytes[1])
			if raw > 1023 {
				raw = 1023
			}
			pct := int(math.Round(float64(raw) / 1023.0 * 100.0))
			eventHub.Broadcast(map[string]any{"tps": pct, "timestamp": timestamp})
		}

	case COOLANT_DID: // Coolant °C
		if len(dataBytes) >= 2 {
			val := int(dataBytes[0])<<8 | int(dataBytes[1])
			eventHub.Broadcast(map[string]any{"coolant": val + COOLANT_OFFSET, "timestamp": timestamp})
		} else if len(dataBytes) == 1 {
			eventHub.Broadcast(map[string]any{"coolant": int(dataBytes[0]) + COOLANT_OFFSET, "timestamp": timestamp})
		}
	}
}
