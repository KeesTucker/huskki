package web

import (
	"crypto/rand"
	"encoding/hex"
)

// generateClientID creates a random identifier used to distinguish browser connections.
func generateClientID() string {
	var randomBytes [16]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(randomBytes[:])
}
