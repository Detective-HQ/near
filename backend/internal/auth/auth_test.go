package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"universal-defi-intelligence/internal/encoding/base58"
)

func TestAuthManager_NonceLifecycle(t *testing.T) {
	mgr := NewManager(Config{
		AuthRequired: true,
		NonceTTL:     5 * time.Minute,
		SessionTTL:   1 * time.Hour,
	})

	nonce, err := mgr.GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce failed: %v", err)
	}
	if len(nonce) != 32 {
		t.Fatalf("Expected 32 hex chars, got %d", len(nonce))
	}

	// First consume should succeed
	if !mgr.ConsumeNonce(nonce) {
		t.Fatalf("Expected nonce to be consumed successfully")
	}

	// Second consume should fail (single use)
	if mgr.ConsumeNonce(nonce) {
		t.Fatalf("Expected consumed nonce to fail on replay")
	}
}

func TestAuthManager_SolanaVerification(t *testing.T) {
	mgr := NewManager(Config{
		AuthRequired: true,
		NonceTTL:     5 * time.Minute,
		SessionTTL:   1 * time.Hour,
	})

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	walletAddress := base58.Encode(pub)
	nonce, err := mgr.GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce failed: %v", err)
	}

	message := "Sign in to Universal DeFi Intelligence with nonce: " + nonce
	sig := ed25519.Sign(priv, []byte(message))
	sigHex := "0x" + hex.EncodeToString(sig)

	sess, token, err := mgr.VerifyWalletSignature(VerifyRequest{
		Chain:         "solana",
		WalletAddress: walletAddress,
		Nonce:         nonce,
		Signature:     sigHex,
		Message:       message,
	})
	if err != nil {
		t.Fatalf("VerifyWalletSignature failed: %v", err)
	}
	if sess == nil || token == "" {
		t.Fatalf("Expected valid session and token, got nil")
	}
	if sess.WalletAddress != walletAddress {
		t.Fatalf("Wallet address mismatch: %s != %s", sess.WalletAddress, walletAddress)
	}

	// Lookup session
	retrieved, err := mgr.GetSession(token)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if retrieved.ID != sess.ID {
		t.Fatalf("Session ID mismatch")
	}

	// Revoke
	mgr.RevokeSession(token)
	if _, err := mgr.GetSession(token); err == nil {
		t.Fatalf("Expected session lookup to fail after revocation")
	}
}

func TestAuthManager_OwnershipCheck(t *testing.T) {
	mgrRequired := NewManager(Config{AuthRequired: true})
	mgrOptional := NewManager(Config{AuthRequired: false})

	sess := &Session{
		ID:            "sess_123",
		WalletAddress: "0x1111111111111111111111111111111111111111",
		Chain:         "ethereum",
	}

	// Optional auth allows all
	if !mgrOptional.CheckOwnership(nil, "other", "0x2222222222222222222222222222222222222222") {
		t.Fatalf("Optional auth should allow all requests")
	}

	// Required auth denies unauthenticated
	if mgrRequired.CheckOwnership(nil, "sess_123", "") {
		t.Fatalf("Required auth should reject nil session")
	}

	// Required auth allows matching session ID
	if !mgrRequired.CheckOwnership(sess, "sess_123", "0x9999999999999999999999999999999999999999") {
		t.Fatalf("Should allow when session ID matches")
	}

	// Required auth allows matching recipient address (case-insensitive)
	if !mgrRequired.CheckOwnership(sess, "other", "0x1111111111111111111111111111111111111111") {
		t.Fatalf("Should allow when recipient matches wallet address")
	}

	// Required auth rejects mismatch
	if mgrRequired.CheckOwnership(sess, "other", "0x3333333333333333333333333333333333333333") {
		t.Fatalf("Should reject when neither session ID nor wallet matches")
	}
}

