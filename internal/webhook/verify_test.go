package webhook

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

const testSecret = "test-secret-value"

func signBody(t *testing.T, secret, timestamp string, body []byte) string {
	t.Helper()
	_, privateKey, err := deriveKeyPair(secret)
	if err != nil {
		t.Fatalf("deriveKeyPair: %v", err)
	}
	message := append([]byte(timestamp), body...)
	return hex.EncodeToString(ed25519.Sign(privateKey, message))
}

func newVerifyRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	verify, err := Verify(testSecret)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	router := gin.New()
	router.POST("/webhook", verify, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return router, testSecret
}

func TestVerifyAcceptsValidSignature(t *testing.T) {
	router, secret := newVerifyRouter(t)
	body := []byte(`{"op":13,"d":{"plain_token":"pt","event_ts":"123"}}`)
	timestamp := "1700000000"

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Signature-Ed25519", signBody(t, secret, timestamp, body))
	req.Header.Set("X-Signature-Timestamp", timestamp)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestVerifyRejectsMissingHeaders(t *testing.T) {
	router, _ := newVerifyRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte(`{}`)))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestVerifyRejectsTamperedBody(t *testing.T) {
	router, secret := newVerifyRouter(t)
	original := []byte(`{"op":0,"t":"C2C_MESSAGE_CREATE"}`)
	tampered := []byte(`{"op":0,"t":"GROUP_AT_MESSAGE_CREATE"}`)
	timestamp := "1700000000"

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(tampered))
	req.Header.Set("X-Signature-Ed25519", signBody(t, secret, timestamp, original))
	req.Header.Set("X-Signature-Timestamp", timestamp)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestSignValidationRoundTrip(t *testing.T) {
	const (
		eventTS    = "1700000000"
		plainToken = "plain-token-value"
	)
	signatureHex, err := SignValidation(testSecret, eventTS, plainToken)
	if err != nil {
		t.Fatalf("SignValidation: %v", err)
	}
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	publicKey, _, err := deriveKeyPair(testSecret)
	if err != nil {
		t.Fatalf("deriveKeyPair: %v", err)
	}
	if !ed25519.Verify(publicKey, []byte(eventTS+plainToken), signature) {
		t.Fatal("signature did not verify")
	}
}
