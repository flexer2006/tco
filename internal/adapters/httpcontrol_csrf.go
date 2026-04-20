package adapters

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	csrfCookieName = "collector_csrf"
	csrfFormField  = "csrf_token"
)

var (
	errInvalidCSRFToken = errors.New("invalid csrf token")
	readCSRFEntropy     = rand.Read
)

func parseAndValidateCSRF(r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return fmt.Errorf("parse form: %w", err)
	}
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil {
		return errInvalidCSRFToken
	}
	formToken, cookieToken := strings.TrimSpace(r.FormValue(csrfFormField)), strings.TrimSpace(cookie.Value)
	if formToken == "" || cookieToken == "" {
		return errInvalidCSRFToken
	}
	if subtle.ConstantTimeCompare([]byte(formToken), []byte(cookieToken)) != 1 {
		return errInvalidCSRFToken
	}
	return nil
}

func generateCSRFToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := readCSRFEntropy(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
