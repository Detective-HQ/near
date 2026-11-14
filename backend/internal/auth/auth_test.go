package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"

	"universal-defi-intelligence/internal/chains/evm"
	"universal-defi-intelligence/internal/encoding/base58"
)

var ctx = context.Background()

func signInMessage(nonce string) string {
	return "Sign in to Universal DeFi Intelligence\nNonce: " + nonce
}

type evmWallet struct {
	key     *secp256k1.PrivateKey
	address string
}

func newEVMWallet(t *testing.T) evmWallet {
	t.Helper()
	key, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha3.NewLegacyKeccak256()
	hash.Write(key.PubKey().SerializeUncompressed()[1:])
	address, err := evm.Checksum(hash.Sum(nil)[12:])
	if err != nil {
		t.Fatal(err)
	}
	return evmWallet{key: key, address: address}
}

// sign returns a 0x-prefixed r||s||v personal_sign signature with v in {27,28}.
func (w evmWallet) sign(message string) string {
	compact := ecdsa.SignCompact(w.key, PersonalSignDigest([]byte(message)), false)
	sig := make([]byte, 65)
	copy(sig, compact[1:])
	sig[64] = compact[0]
	return "0x" + hex.EncodeToString(sig)
}

func TestNonceIsSingleUse(t *testing.T) {
	mgr := NewManager(Config{})
	wallet := newEVMWallet(t)
	nonce, err := mgr.GenerateNonce(ctx)
	if err != nil || len(nonce) != 32 {
		t.Fatalf("GenerateNonce = %q, %v", nonce, err)
	}
	message := signInMessage(nonce)
	req := VerifyRequest{Chain: "ethereum", WalletAddress: wallet.address, Nonce: nonce, Message: message, Signature: wallet.sign(message)}
	if _, _, err := mgr.VerifyWalletSignature(ctx, req); err != nil {
		t.Fatalf("first verify: %v", err)
	}
	if _, _, err := mgr.VerifyWalletSignature(ctx, req); !errors.Is(err, ErrInvalidNonce) {
		t.Fatalf("replay err = %v, want ErrInvalidNonce", err)
	}
}

func TestEVMSignatureIsCryptographicallyVerified(t *testing.T) {
	mgr := NewManager(Config{})
	wallet := newEVMWallet(t)
	attacker := newEVMWallet(t)

	nonce, _ := mgr.GenerateNonce(ctx)
	message := signInMessage(nonce) + "\nAddress: " + wallet.address
	session, token, err := mgr.VerifyWalletSignature(ctx, VerifyRequest{
		Chain: "base", WalletAddress: strings.ToLower(wallet.address), Nonce: nonce,
		Message: message, Signature: wallet.sign(message),
	})
	if err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if session.WalletAddress != wallet.address || session.Chain != "evm" || token == "" {
		t.Fatalf("unexpected session %+v", session)
	}
	if strings.HasPrefix(token, session.ID) {
		t.Fatal("session id must not be derived from the bearer token")
	}

	// Signed by someone else but claiming the victim's address.
	nonce, _ = mgr.GenerateNonce(ctx)
	message = signInMessage(nonce) + "\nAddress: " + wallet.address
	if _, _, err := mgr.VerifyWalletSignature(ctx, VerifyRequest{
		Chain: "ethereum", WalletAddress: wallet.address, Nonce: nonce,
		Message: message, Signature: attacker.sign(message),
	}); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("forged signature err = %v, want ErrInvalidSignature", err)
	}

	// Arbitrary 65 bytes must not authenticate.
	nonce, _ = mgr.GenerateNonce(ctx)
	message = signInMessage(nonce) + wallet.address
	if _, _, err := mgr.VerifyWalletSignature(ctx, VerifyRequest{
		Chain: "ethereum", WalletAddress: wallet.address, Nonce: nonce,
		Message: message, Signature: "0x" + strings.Repeat("11", 65),
	}); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("garbage signature err = %v, want ErrInvalidSignature", err)
	}
}

func TestSignedMessageMustContainNonce(t *testing.T) {
	mgr := NewManager(Config{})
	wallet := newEVMWallet(t)
	nonce, _ := mgr.GenerateNonce(ctx)
	message := "an old message signed for some other site"
	if _, _, err := mgr.VerifyWalletSignature(ctx, VerifyRequest{
		Chain: "ethereum", WalletAddress: wallet.address, Nonce: nonce,
		Message: message, Signature: wallet.sign(message),
	}); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err = %v, want ErrInvalidSignature", err)
	}
}

func TestSolanaVerificationAndSessionLifecycle(t *testing.T) {
	mgr := NewManager(Config{AuthRequired: true, SessionTTL: time.Hour})
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	wallet := base58.Encode(pub)
	nonce, _ := mgr.GenerateNonce(ctx)
	message := signInMessage(nonce)

	session, token, err := mgr.VerifyWalletSignature(ctx, VerifyRequest{
		Chain: "solana", WalletAddress: wallet, Nonce: nonce, Message: message,
		Signature: base58.Encode(ed25519.Sign(priv, []byte(message))),
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	got, err := mgr.GetSession(ctx, token)
	if err != nil || got.ID != session.ID {
		t.Fatalf("GetSession = %+v, %v", got, err)
	}
	if err := mgr.RevokeSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GetSession(ctx, token); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("after revoke err = %v", err)
	}
}

type fakeAccessKeys struct{ owner, key string }

func (f fakeAccessKeys) VerifyAccessKey(_ context.Context, account, key string) error {
	if account == f.owner && key == f.key {
		return nil
	}
	return errors.New("not a key of this account")
}

func TestNEARKeyMustBelongToAccount(t *testing.T) {
	victimPub, _, _ := ed25519.GenerateKey(rand.Reader)
	attackerPub, attackerPriv, _ := ed25519.GenerateKey(rand.Reader)
	mgr := NewManager(Config{NEARKeys: fakeAccessKeys{owner: "victim.near", key: base58.Encode(victimPub)}})

	verify := func(account string) error {
		nonce, _ := mgr.GenerateNonce(ctx)
		message := signInMessage(nonce)
		_, _, err := mgr.VerifyWalletSignature(ctx, VerifyRequest{
			Chain: "near", WalletAddress: account, Nonce: nonce, Message: message,
			PublicKey: "ed25519:" + base58.Encode(attackerPub),
			Signature: base58.Encode(ed25519.Sign(attackerPriv, []byte(message))),
		})
		return err
	}

	if err := verify("victim.near"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("named account takeover err = %v", err)
	}
	if err := verify(hex.EncodeToString(victimPub)); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("implicit account takeover err = %v", err)
	}
	if err := verify(hex.EncodeToString(attackerPub)); err != nil {
		t.Fatalf("own implicit account rejected: %v", err)
	}
}

func TestCanAccess(t *testing.T) {
	optional := NewManager(Config{})
	required := NewManager(Config{AuthRequired: true})
	owner := &Session{WalletAddress: "0x1111111111111111111111111111111111111111", Chain: "evm"}
	other := &Session{WalletAddress: "0x2222222222222222222222222222222222222222", Chain: "evm"}

	if !optional.CanAccess(nil, "") {
		t.Fatal("anonymous record should be readable when auth is optional")
	}
	if required.CanAccess(nil, "") {
		t.Fatal("auth required must reject anonymous callers")
	}
	if optional.CanAccess(nil, owner.Principal()) {
		t.Fatal("owned record must not be readable anonymously, even when auth is optional")
	}
	if optional.CanAccess(other, owner.Principal()) || required.CanAccess(other, owner.Principal()) {
		t.Fatal("another wallet must not access the record")
	}
	if !required.CanAccess(owner, owner.Principal()) {
		t.Fatal("owner must access their record")
	}
}

func TestRecoverRejectsHighS(t *testing.T) {
	wallet := newEVMWallet(t)
	raw, _ := hex.DecodeString(strings.TrimPrefix(wallet.sign("hello"), "0x"))
	// s' = n - s flips to the high-s twin, which must be rejected.
	var s, n secp256k1.ModNScalar
	s.SetByteSlice(raw[32:64])
	n.NegateVal(&s)
	high := n.Bytes()
	copy(raw[32:64], high[:])
	if _, err := RecoverEVMAddress([]byte("hello"), raw); err == nil {
		t.Fatal("high-s signature accepted")
	}
}

func TestVerifyAcceptsChainFamilyAndCanonicalMessage(t *testing.T) {
	mgr := NewManager(Config{})
	wallet := newEVMWallet(t)
	nonce, _ := mgr.GenerateNonce(ctx)
	// The wallet signs the canonical message; the client omits "message".
	sig := wallet.sign(SignInMessage(nonce))
	session, _, err := mgr.VerifyWalletSignature(ctx, VerifyRequest{
		ChainFamily: "evm", WalletAddress: wallet.address, Nonce: nonce, Signature: sig,
	})
	if err != nil || session.WalletAddress != wallet.address {
		t.Fatalf("family/canonical verify = %+v, %v", session, err)
	}

	// A signature over different text must not verify against the canonical message.
	nonce2, _ := mgr.GenerateNonce(ctx)
	other := wallet.sign("something else " + nonce2)
	if _, _, err := mgr.VerifyWalletSignature(ctx, VerifyRequest{
		ChainFamily: "evm", WalletAddress: wallet.address, Nonce: nonce2, Signature: other,
	}); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("mismatched signature err = %v", err)
	}
}

func TestNEP413Verification(t *testing.T) {
	mgr := NewManager(Config{})
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	accountID := hex.EncodeToString(pub) // implicit account
	nonce, _ := mgr.GenerateNonce(ctx)
	msg := SignInMessage(nonce)
	digest := nep413Digest(msg, nonce, accountID)
	sig := ed25519.Sign(priv, digest)

	session, _, err := mgr.VerifyWalletSignature(ctx, VerifyRequest{
		ChainFamily:   "near",
		WalletAddress: accountID,
		Nonce:         nonce,
		Message:       msg,
		Signature:     base58.Encode(sig),
		PublicKey:     "ed25519:" + base58.Encode(pub),
	})
	if err != nil {
		t.Fatalf("NEP-413 verification failed: %v", err)
	}
	if session.WalletAddress != accountID {
		t.Fatalf("session address = %s, want %s", session.WalletAddress, accountID)
	}
}
