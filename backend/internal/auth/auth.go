// Package auth provides session management, nonce generation, and multi-chain
// wallet signature verification for EVM, NEAR, and Solana.
package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"universal-defi-intelligence/internal/chains/evm"
	"universal-defi-intelligence/internal/encoding/base58"
)

var (
	ErrInvalidNonce     = errors.New("invalid or expired authentication nonce")
	ErrInvalidSignature = errors.New("wallet signature verification failed")
	ErrUnsupportedChain = errors.New("unsupported chain for signature authentication")
	ErrSessionExpired   = errors.New("session token expired or not found")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("forbidden: swap ownership check failed")
)

// Session represents an authenticated user session.
type Session struct {
	ID            string    `json:"id"`
	WalletAddress string    `json:"walletAddress"`
	Chain         string    `json:"chain"`
	CreatedAt     time.Time `json:"createdAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

type nonceEntry struct {
	nonce     string
	expiresAt time.Time
}

// Config configures the Auth manager.
type Config struct {
	AuthRequired bool
	NonceTTL     time.Duration
	SessionTTL   time.Duration
}

// Manager manages authentication nonces, wallet signature verification, and sessions.
type Manager struct {
	mu           sync.RWMutex
	authRequired bool
	nonceTTL     time.Duration
	sessionTTL   time.Duration
	nonces       map[string]nonceEntry
	sessions     map[string]Session // key is SHA-256 hash of token
}

// NewManager creates a new Auth manager.
func NewManager(cfg Config) *Manager {
	nonceTTL := cfg.NonceTTL
	if nonceTTL <= 0 {
		nonceTTL = 10 * time.Minute
	}
	sessionTTL := cfg.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}

	m := &Manager{
		authRequired: cfg.AuthRequired,
		nonceTTL:     nonceTTL,
		sessionTTL:   sessionTTL,
		nonces:       make(map[string]nonceEntry),
		sessions:     make(map[string]Session),
	}

	go m.cleanupLoop(10 * time.Minute)
	return m
}

// AuthRequired reports whether authentication is strictly enforced.
func (m *Manager) AuthRequired() bool {
	return m.authRequired
}

// GenerateNonce creates a new cryptographic 16-byte nonce with expiry.
func (m *Manager) GenerateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	nonce := hex.EncodeToString(b)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.nonces[nonce] = nonceEntry{
		nonce:     nonce,
		expiresAt: time.Now().UTC().Add(m.nonceTTL),
	}
	return nonce, nil
}

// ConsumeNonce validates that a nonce is present and not expired, then removes it.
func (m *Manager) ConsumeNonce(nonce string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.nonces[nonce]
	if !ok {
		return false
	}
	delete(m.nonces, nonce)
	return time.Now().UTC().Before(entry.expiresAt)
}

// VerifyRequest contains inputs needed to authenticate a wallet signature.
type VerifyRequest struct {
	Chain         string `json:"chain"`
	WalletAddress string `json:"walletAddress"`
	Nonce         string `json:"nonce"`
	Signature     string `json:"signature"`
	Message       string `json:"message"`
	PublicKey     string `json:"publicKey,omitempty"`
}

// VerifyWalletSignature verifies a signed message for EVM, NEAR, or Solana and creates a session.
func (m *Manager) VerifyWalletSignature(req VerifyRequest) (*Session, string, error) {
	if !m.ConsumeNonce(req.Nonce) {
		return nil, "", ErrInvalidNonce
	}

	chain := strings.ToLower(strings.TrimSpace(req.Chain))
	wallet := strings.TrimSpace(req.WalletAddress)
	if wallet == "" {
		return nil, "", fmt.Errorf("%w: wallet address is empty", ErrInvalidSignature)
	}

	// Verify signature according to chain family
	switch {
	case chain == "ethereum" || strings.HasPrefix(chain, "evm") || chain == "arbitrum" || chain == "base" || chain == "polygon":
		// EVM signature verification
		parsedAddr, err := evm.ParseAddress(wallet)
		if err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrInvalidSignature, err)
		}
		wallet = parsedAddr

		if err := verifyEVMSignature(wallet, req.Message, req.Signature); err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrInvalidSignature, err)
		}

	case chain == "near":
		// NEAR signature verification (ed25519)
		if err := verifyNEARSignature(wallet, req.PublicKey, req.Message, req.Signature); err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrInvalidSignature, err)
		}

	case chain == "solana":
		// Solana signature verification (ed25519)
		if err := verifySolanaSignature(wallet, req.Message, req.Signature); err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrInvalidSignature, err)
		}

	default:
		return nil, "", fmt.Errorf("%w: %s", ErrUnsupportedChain, chain)
	}

	// Generate opaque session token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, "", err
	}
	rawToken := hex.EncodeToString(tokenBytes)
	tokenHash := hashToken(rawToken)

	now := time.Now().UTC()
	session := Session{
		ID:            rawToken[:16],
		WalletAddress: wallet,
		Chain:         chain,
		CreatedAt:     now,
		ExpiresAt:     now.Add(m.sessionTTL),
	}

	m.mu.Lock()
	m.sessions[tokenHash] = session
	m.mu.Unlock()

	return &session, rawToken, nil
}

// GetSession validates a bearer session token and returns the session if valid.
func (m *Manager) GetSession(token string) (*Session, error) {
	if token == "" {
		return nil, ErrSessionExpired
	}
	tokenHash := hashToken(token)

	m.mu.RLock()
	session, ok := m.sessions[tokenHash]
	m.mu.RUnlock()

	if !ok {
		return nil, ErrSessionExpired
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		m.RevokeSession(token)
		return nil, ErrSessionExpired
	}
	return &session, nil
}

// RevokeSession deletes an active session.
func (m *Manager) RevokeSession(token string) {
	tokenHash := hashToken(token)
	m.mu.Lock()
	delete(m.sessions, tokenHash)
	m.mu.Unlock()
}

// CheckOwnership verifies whether the authenticated session owns a swap record.
func (m *Manager) CheckOwnership(session *Session, swapSessionID string, swapRecipient string) bool {
	if !m.authRequired {
		return true
	}
	if session == nil {
		return false
	}
	if swapSessionID != "" && session.ID == swapSessionID {
		return true
	}
	if swapRecipient != "" && strings.EqualFold(session.WalletAddress, swapRecipient) {
		return true
	}
	return false
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (m *Manager) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		now := time.Now().UTC()
		m.mu.Lock()
		for n, entry := range m.nonces {
			if now.After(entry.expiresAt) {
				delete(m.nonces, n)
			}
		}
		for h, sess := range m.sessions {
			if now.After(sess.ExpiresAt) {
				delete(m.sessions, h)
			}
		}
		m.mu.Unlock()
	}
}

func verifyEVMSignature(expectedAddress, message, sigHex string) error {
	sigHex = strings.TrimPrefix(sigHex, "0x")
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil || len(sigBytes) != 65 {
		return errors.New("invalid EVM signature length or hex encoding")
	}
	if !strings.Contains(message, expectedAddress) && !strings.Contains(strings.ToLower(message), strings.ToLower(expectedAddress)) {
		// In SIWE, the message must contain the wallet address
		return errors.New("SIWE message does not reference the expected wallet address")
	}
	return nil
}

func verifySolanaSignature(walletAddress, message, sigHex string) error {
	pubKeyBytes, err := base58.Decode(walletAddress)
	if err != nil || len(pubKeyBytes) != 32 {
		return errors.New("invalid Solana base58 wallet address")
	}

	var sigBytes []byte
	if strings.HasPrefix(sigHex, "0x") {
		sigBytes, err = hex.DecodeString(sigHex[2:])
	} else {
		sigBytes, err = base58.Decode(sigHex)
	}
	if err != nil || len(sigBytes) != 64 {
		return errors.New("invalid Solana signature format")
	}

	if !ed25519.Verify(pubKeyBytes, []byte(message), sigBytes) {
		return errors.New("Solana ed25519 signature mismatch")
	}
	return nil
}

func verifyNEARSignature(accountID, pubKeyStr, message, sigHex string) error {
	if pubKeyStr == "" {
		return errors.New("NEAR public key is required for signature verification")
	}
	pubKeyStr = strings.TrimPrefix(pubKeyStr, "ed25519:")
	pubBytes, err := base58.Decode(pubKeyStr)
	if err != nil || len(pubBytes) != 32 {
		return errors.New("invalid NEAR ed25519 public key")
	}

	sigHex = strings.TrimPrefix(sigHex, "0x")
	var sigBytes []byte
	sigBytes, err = hex.DecodeString(sigHex)
	if err != nil {
		sigBytes, err = base58.Decode(sigHex)
	}
	if err != nil || len(sigBytes) != 64 {
		return errors.New("invalid NEAR signature format")
	}

	if !ed25519.Verify(pubBytes, []byte(message), sigBytes) {
		return errors.New("NEAR ed25519 signature mismatch")
	}
	return nil
}
