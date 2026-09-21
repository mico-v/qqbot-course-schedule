// Package webhook verifies and dispatches QQ open platform webhook events.
package webhook

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// OpValidation is the ping/validation opcode sent when a callback URL is saved.
const OpValidation = 13

// deriveKeyPair reproduces the platform's ed25519 key derivation from the bot secret.
func deriveKeyPair(secret string) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	if secret == "" {
		return nil, nil, errors.New("secret 不能为空")
	}
	seed := secret
	for len(seed) < ed25519.SeedSize {
		seed += secret
	}
	publicKey, privateKey, err := ed25519.GenerateKey(strings.NewReader(seed[:ed25519.SeedSize]))
	if err != nil {
		return nil, nil, err
	}
	return publicKey, privateKey, nil
}

// SignValidation returns the hex signature the platform expects for op=13.
func SignValidation(secret, eventTS, plainToken string) (string, error) {
	_, privateKey, err := deriveKeyPair(secret)
	if err != nil {
		return "", err
	}
	message := []byte(eventTS + plainToken)
	return hex.EncodeToString(ed25519.Sign(privateKey, message)), nil
}

// Verify validates the X-Signature-Ed25519 header against the raw request body.
// The body is restored so downstream handlers can read it again.
func Verify(secret string) (gin.HandlerFunc, error) {
	publicKey, _, err := deriveKeyPair(secret)
	if err != nil {
		return nil, err
	}
	return func(c *gin.Context) {
		signatureHex := c.GetHeader("X-Signature-Ed25519")
		timestamp := c.GetHeader("X-Signature-Timestamp")
		if signatureHex == "" || timestamp == "" {
			slog.Warn("webhook rejected: missing signature headers", "path", c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing signature"})
			return
		}
		signature, err := hex.DecodeString(signatureHex)
		if err != nil || len(signature) != ed25519.SignatureSize {
			slog.Warn("webhook rejected: malformed signature")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "malformed signature"})
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "read body"})
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		message := make([]byte, 0, len(timestamp)+len(body))
		message = append(message, timestamp...)
		message = append(message, body...)
		if !ed25519.Verify(publicKey, message, signature) {
			slog.Warn("webhook rejected: signature mismatch")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "signature mismatch"})
			return
		}
		c.Next()
	}, nil
}
