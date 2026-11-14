// Package auth provides wallet sign-in, single-use nonces, and opaque bearer
// sessions for EVM, NEAR, and Solana wallets.
//
// Security properties:
//   - Every signed message must contain a server-issued, single-use nonce, so a
//     signature captured elsewhere cannot be replayed to sign in.
//   - EVM signatures are verified by secp256k1 public-key recovery over the
//     EIP-191 personal_sign digest; the recovered address must match.
//   - NEAR signatures are verified with ed25519 and the signing key must belong
//     to the claimed account (implicit accounts by derivation, named accounts
//     through an on-chain access-key lookup).
//   - Solana signatures are verified with ed25519 against the wallet address.
//   - Bearer tokens are 256-bit random values; only their SHA-256 is stored.
//     The public session id is independent random data, never a token prefix.
package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"

	"universal-defi-intelligence/internal/chains/evm"
	"universal-defi-intelligence/internal/chains/near"
	"universal-defi-intelligence/internal/chains/registry"
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

const maxMessageBytes = 4096

// Session represents an authenticated wallet session.
type Session struct {
	ID            string    `json:"id"`
	WalletAddress string    `json:"walletAddress"`
	Chain         string    `json:"chain"`
	CreatedAt     time.Time `json:"createdAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

// Principal is the stable owner identity recorded on quotes and swaps. It is
// derived from the wallet, not the session, so a user keeps access to their
// swaps after signing in again.
func (s *Session) Principal() string {
	if s == nil || s.WalletAddress == "" {
		return ""
	}
	return strings.ToLower(s.Chain) + ":" + strings.ToLower(s.WalletAddress)
}

// NonceStore issues and atomically consumes single-use nonces.
type NonceStore interface {
	Put(ctx context.Context, nonce string, expiresAt time.Time) error
	// Consume deletes the nonce and reports whether it existed and was unexpired.
	Consume(ctx context.Context, nonce string, now time.Time) (bool, error)
}

// SessionStore persists sessions keyed by the SHA-256 of the bearer token.
type SessionStore interface {
	Save(ctx context.Context, tokenHash string, session Session) error
	Get(ctx context.Context, tokenHash string) (Session, bool, error)
	Delete(ctx context.Context, tokenHash string) error
}

// AccessKeyVerifier confirms that an ed25519 public key (base58, no prefix) is
// a full-access key of a NEAR named account.
type AccessKeyVerifier interface {
	VerifyAccessKey(ctx context.Context, accountID, publicKeyBase58 string) error
}

// Config configures the Auth manager.
type Config struct {
	AuthRequired bool
	NonceTTL     time.Duration
	SessionTTL   time.Duration
	Registry     *registry.Registry
	Nonces       NonceStore
	Sessions     SessionStore
	NEARKeys     AccessKeyVerifier
	Now          func() time.Time
}

// Manager manages nonces, wallet signature verification, and sessions.
type Manager struct {
	authRequired bool
	nonceTTL     time.Duration
	sessionTTL   time.Duration
	registry     *registry.Registry
	nonces       NonceStore
	sessions     SessionStore
	nearKeys     AccessKeyVerifier
	now          func() time.Time
}

// NewManager creates an Auth manager. Nil stores fall back to in-memory ones,
// which are suitable for a single API instance or tests.
func NewManager(cfg Config) *Manager {
	nonceTTL := cfg.NonceTTL
	if nonceTTL <= 0 {
		nonceTTL = 10 * time.Minute
	}
	sessionTTL := cfg.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}
	reg := cfg.Registry
	if reg == nil {
		reg, _ = registry.Load()
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	memory := newMemoryStore()
	nonces := cfg.Nonces
	if nonces == nil {
		nonces = memory
	}
	sessions := cfg.Sessions
	if sessions == nil {
		sessions = memory
	}
	return &Manager{
		authRequired: cfg.AuthRequired,
		nonceTTL:     nonceTTL,
		sessionTTL:   sessionTTL,
		registry:     reg,
		nonces:       nonces,
		sessions:     sessions,
		nearKeys:     cfg.NEARKeys,
		now:          now,
	}
}

// AuthRequired reports whether authentication is strictly enforced.
func (m *Manager) AuthRequired() bool {
	return m.authRequired
}

// GenerateNonce creates a single-use 128-bit nonce.
func (m *Manager) GenerateNonce(ctx context.Context) (string, error) {
	nonce, err := randomHex(16)
	if err != nil {
		return "", err
	}
	if err := m.nonces.Put(ctx, nonce, m.now().Add(m.nonceTTL)); err != nil {
		return "", err
	}
	return nonce, nil
}

// SignInMessage is the canonical text a wallet signs for a nonce. The nonce
// endpoint returns it, and a verify request that omits "message" is checked
// against exactly this text.
func SignInMessage(nonce string) string {
	return "Sign in to Universal DeFi Intelligence\nNonce: " + nonce
}

// familyChains maps chain-family names (as sent by wallet UIs) to a registry
// chain of that family. Verification only depends on the family.
var familyChains = map[string]string{"evm": "ethereum", "near": "near", "solana": "solana"}

// VerifyRequest contains inputs needed to authenticate a wallet signature.
type VerifyRequest struct {
	Chain string `json:"chain"`
	// ChainFamily is accepted as an alias of Chain ("evm", "near", "solana").
	ChainFamily   string `json:"chainFamily,omitempty"`
	WalletAddress string `json:"walletAddress"`
	Nonce         string `json:"nonce"`
	Signature     string `json:"signature"`
	Message       string `json:"message"`
	PublicKey     string `json:"publicKey,omitempty"`
}

// VerifyWalletSignature verifies a signed sign-in message and creates a
// session. The returned token is shown to the client exactly once.
func (m *Manager) VerifyWalletSignature(ctx context.Context, req VerifyRequest) (*Session, string, error) {
	nonce := strings.TrimSpace(req.Nonce)
	if req.Message == "" && nonce != "" {
		req.Message = SignInMessage(nonce)
	}
	if nonce == "" || len(req.Message) > maxMessageBytes {
		return nil, "", ErrInvalidNonce
	}
	// The nonce must be part of what the wallet signed; otherwise any old
	// signature over an unrelated message would be accepted.
	if !strings.Contains(req.Message, nonce) {
		return nil, "", fmt.Errorf("%w: signed message does not contain the nonce", ErrInvalidSignature)
	}

	chainKey := strings.TrimSpace(req.Chain)
	if chainKey == "" {
		chainKey = strings.TrimSpace(req.ChainFamily)
	}
	if mapped, isFamily := familyChains[strings.ToLower(chainKey)]; isFamily {
		if _, known := m.registry.Get(chainKey); !known {
			chainKey = mapped
		}
	}
	chain, ok := m.registry.Get(chainKey)
	if !ok {
		return nil, "", fmt.Errorf("%w: %s", ErrUnsupportedChain, req.Chain)
	}
	wallet := strings.TrimSpace(req.WalletAddress)
	if wallet == "" {
		return nil, "", fmt.Errorf("%w: wallet address is empty", ErrInvalidSignature)
	}

	// Consume before verifying so a failed attempt still burns the nonce and
	// signatures cannot be brute-forced against one nonce.
	valid, err := m.nonces.Consume(ctx, nonce, m.now())
	if err != nil {
		return nil, "", err
	}
	if !valid {
		return nil, "", ErrInvalidNonce
	}

	switch chain.Family {
	case registry.FamilyEVM:
		normalized, parseErr := evm.ParseAddress(wallet)
		if parseErr != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrInvalidSignature, parseErr)
		}
		if err := verifyEVMSignature(normalized, req.Message, req.Signature); err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrInvalidSignature, err)
		}
		wallet = normalized
	case registry.FamilyNEAR:
		account, parseErr := near.ParseAccountID(wallet)
		if parseErr != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrInvalidSignature, parseErr)
		}
		if err := m.verifyNEARSignature(ctx, account, req.PublicKey, req.Message, req.Signature); err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrInvalidSignature, err)
		}
		wallet = account
	case registry.FamilySolana:
		if err := verifySolanaSignature(wallet, req.Message, req.Signature); err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrInvalidSignature, err)
		}
	default:
		return nil, "", fmt.Errorf("%w: %s", ErrUnsupportedChain, chain.Key)
	}

	rawToken, err := randomHex(32)
	if err != nil {
		return nil, "", err
	}
	sessionID, err := randomHex(16)
	if err != nil {
		return nil, "", err
	}
	now := m.now()
	session := Session{
		ID:            sessionID,
		WalletAddress: wallet,
		Chain:         string(chain.Family),
		CreatedAt:     now,
		ExpiresAt:     now.Add(m.sessionTTL),
	}
	if err := m.sessions.Save(ctx, hashToken(rawToken), session); err != nil {
		return nil, "", err
	}
	return &session, rawToken, nil
}

// GetSession validates a bearer token and returns its session.
func (m *Manager) GetSession(ctx context.Context, token string) (*Session, error) {
	if token == "" || len(token) > 256 {
		return nil, ErrSessionExpired
	}
	hash := hashToken(token)
	session, ok, err := m.sessions.Get(ctx, hash)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrSessionExpired
	}
	if !m.now().Before(session.ExpiresAt) {
		_ = m.sessions.Delete(ctx, hash)
		return nil, ErrSessionExpired
	}
	return &session, nil
}

// RevokeSession deletes an active session.
func (m *Manager) RevokeSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return m.sessions.Delete(ctx, hashToken(token))
}

// CanAccess reports whether a caller may read or mutate a record owned by
// ownerPrincipal. Records created anonymously (empty owner) stay accessible
// when authentication is optional; records with an owner are only accessible
// to that owner, regardless of the AuthRequired flag.
func (m *Manager) CanAccess(session *Session, ownerPrincipal string) bool {
	if ownerPrincipal == "" {
		return !m.authRequired || session != nil
	}
	return session != nil && strings.EqualFold(session.Principal(), ownerPrincipal)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// verifyEVMSignature recovers the signer of an EIP-191 personal_sign message
// and compares it with the expected checksummed address.
func verifyEVMSignature(expectedAddress, message, sigHex string) error {
	sig, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(sigHex), "0x"))
	if err != nil || len(sig) != 65 {
		return errors.New("signature must be 65 bytes of hex")
	}
	recovered, err := RecoverEVMAddress([]byte(message), sig)
	if err != nil {
		return err
	}
	if !strings.EqualFold(recovered, expectedAddress) {
		return errors.New("recovered signer does not match the wallet address")
	}
	return nil
}

// PersonalSignDigest returns keccak256("\x19Ethereum Signed Message:\n" + len + message).
func PersonalSignDigest(message []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write([]byte("\x19Ethereum Signed Message:\n" + strconv.Itoa(len(message))))
	hash.Write(message)
	return hash.Sum(nil)
}

// RecoverEVMAddress recovers the checksummed signer address from a 65-byte
// r||s||v personal_sign signature (v may be 0/1 or 27/28).
func RecoverEVMAddress(message, sig []byte) (string, error) {
	if len(sig) != 65 {
		return "", errors.New("signature must be 65 bytes")
	}
	v := sig[64]
	if v >= 27 {
		v -= 27
	}
	if v > 1 {
		return "", errors.New("invalid signature recovery id")
	}
	// Reject high-s signatures (EIP-2) so each message has one valid signature.
	if !lowS(sig[32:64]) {
		return "", errors.New("signature s value is not canonical")
	}
	compact := make([]byte, 65)
	compact[0] = 27 + v
	copy(compact[1:], sig[:64])
	pub, _, err := ecdsa.RecoverCompact(compact, PersonalSignDigest(message))
	if err != nil {
		return "", fmt.Errorf("recover public key: %w", err)
	}
	uncompressed := pub.SerializeUncompressed()
	hash := sha3.NewLegacyKeccak256()
	hash.Write(uncompressed[1:])
	return evm.Checksum(hash.Sum(nil)[12:])
}

// secp256k1 half order, big-endian.
var secpHalfOrder = [32]byte{
	0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0x5d, 0x57, 0x6e, 0x73, 0x57, 0xa4, 0x50, 0x1d, 0xdf, 0xe9, 0x2f, 0x46, 0x68, 0x1b, 0x20, 0xa0,
}

func lowS(s []byte) bool {
	for i := 0; i < 32; i++ {
		if s[i] != secpHalfOrder[i] {
			return s[i] < secpHalfOrder[i]
		}
	}
	return true
}

func decodeSignature(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "0x") {
		return hex.DecodeString(value[2:])
	}
	if len(value) == 128 {
		if decoded, err := hex.DecodeString(value); err == nil {
			return decoded, nil
		}
	}
	return base58.Decode(value)
}

func verifySolanaSignature(walletAddress, message, signature string) error {
	pubKey, err := base58.Decode(walletAddress)
	if err != nil || len(pubKey) != ed25519.PublicKeySize {
		return errors.New("invalid solana base58 wallet address")
	}
	sig, err := decodeSignature(signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("invalid solana signature format")
	}
	if !ed25519.Verify(pubKey, []byte(message), sig) {
		return errors.New("solana ed25519 signature mismatch")
	}
	return nil
}

func (m *Manager) verifyNEARSignature(ctx context.Context, accountID, publicKey, message, signature string) error {
	keyB58 := strings.TrimPrefix(strings.TrimSpace(publicKey), "ed25519:")
	if keyB58 == "" {
		return errors.New("near public key is required")
	}
	pubKey, err := base58.Decode(keyB58)
	if err != nil || len(pubKey) != ed25519.PublicKeySize {
		return errors.New("invalid near ed25519 public key")
	}
	sig, err := decodeSignature(signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("invalid near signature format")
	}
	if !ed25519.Verify(pubKey, []byte(message), sig) {
		return errors.New("near ed25519 signature mismatch")
	}

	// The key must belong to the claimed account.
	if near.IsImplicit(accountID) {
		if !strings.EqualFold(hex.EncodeToString(pubKey), accountID) {
			return errors.New("public key does not derive the implicit account")
		}
		return nil
	}
	if m.nearKeys == nil {
		return errors.New("named NEAR accounts require an access-key verifier")
	}
	if err := m.nearKeys.VerifyAccessKey(ctx, accountID, keyB58); err != nil {
		return fmt.Errorf("public key is not a full-access key of %s: %w", accountID, err)
	}
	return nil
}

// memoryStore is the in-process NonceStore and SessionStore.
type memoryStore struct {
	mu       sync.Mutex
	nonces   map[string]time.Time
	sessions map[string]Session
	ops      int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{nonces: map[string]time.Time{}, sessions: map[string]Session{}}
}

func (s *memoryStore) Put(_ context.Context, nonce string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(time.Now().UTC())
	s.nonces[nonce] = expiresAt
	return nil
}

func (s *memoryStore) Consume(_ context.Context, nonce string, now time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiresAt, ok := s.nonces[nonce]
	if !ok {
		return false, nil
	}
	delete(s.nonces, nonce)
	return now.Before(expiresAt), nil
}

func (s *memoryStore) Save(_ context.Context, tokenHash string, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(time.Now().UTC())
	s.sessions[tokenHash] = session
	return nil
}

func (s *memoryStore) Get(_ context.Context, tokenHash string) (Session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[tokenHash]
	return session, ok, nil
}

func (s *memoryStore) Delete(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, tokenHash)
	return nil
}

// sweepLocked drops expired entries every 256 writes, bounding memory without
// a background goroutine.
func (s *memoryStore) sweepLocked(now time.Time) {
	s.ops++
	if s.ops%256 != 0 {
		return
	}
	for nonce, expiresAt := range s.nonces {
		if !now.Before(expiresAt) {
			delete(s.nonces, nonce)
		}
	}
	for hash, session := range s.sessions {
		if !now.Before(session.ExpiresAt) {
			delete(s.sessions, hash)
		}
	}
}
