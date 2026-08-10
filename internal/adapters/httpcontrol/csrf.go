package httpcontrol

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
	csrfHeaderName = "X-Csrf-Token"
	csrfTokenBytes = 32
)

var (
	errInvalidCSRFToken = errors.New("invalid csrf token")
	readCSRFEntropy     = rand.Read
)

func parseAndValidateCSRF(r *http.Request) error {
	err := r.ParseForm()
	if err != nil {
		return fmt.Errorf("parse form: %w", err)
	}

	cookie, err := r.Cookie(csrfCookieName)
	if err != nil {
		return errInvalidCSRFToken
	}

	cookieToken := strings.TrimSpace(cookie.Value)
	if cookieToken == "" {
		return errInvalidCSRFToken
	}

	provided := strings.TrimSpace(r.FormValue(csrfFormField))
	if provided == "" {
		provided = strings.TrimSpace(r.Header.Get(csrfHeaderName))
	}

	if provided == "" {
		return errInvalidCSRFToken
	}

	if subtle.ConstantTimeCompare([]byte(provided), []byte(cookieToken)) != 1 {
		return errInvalidCSRFToken
	}

	return nil
}

func generateCSRFToken() (string, error) {
	raw := make([]byte, csrfTokenBytes)

	_, err := readCSRFEntropy(raw)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(raw), nil
}

func ensureCSRFCookie(w http.ResponseWriter, r *http.Request) (string, error) {
	cookie, err := r.Cookie(csrfCookieName)
	if err == nil {
		token := strings.TrimSpace(cookie.Value)
		if token != "" {
			return token, nil
		}
	}

	token, err := generateCSRFToken()
	if err != nil {
		return "", err
	}

	http.SetCookie(w, new(http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	}))

	return token, nil
}
