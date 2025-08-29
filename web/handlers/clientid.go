package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const clientIDCookieName = "client-id"

// getClientID returns a stable identifier for the client using a cookie.
// If the cookie is missing, it generates a new random identifier and sets it.
func getClientID(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie(clientIDCookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	var randomBytes [16]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		identifier := r.RemoteAddr
		http.SetCookie(w, &http.Cookie{Name: clientIDCookieName, Value: identifier, Path: "/"})
		return identifier
	}
	identifier := hex.EncodeToString(randomBytes[:])
	http.SetCookie(w, &http.Cookie{
		Name:  clientIDCookieName,
		Value: identifier,
		Path:  "/",
	})
	return identifier
}
