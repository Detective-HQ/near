package solana

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"filippo.io/edwards25519"

	"universal-defi-intelligence/internal/encoding/base58"
)

// Well-known Solana program IDs.
const (
	SystemProgramID            = "11111111111111111111111111111111"
	TokenProgramID             = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
	AssociatedTokenProgramID   = "ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"
	MemoProgramID              = "MemoSq4gqABAXKb96qnH8TysNcWxMyWCqXgDLGmfcHr"
	ComputeBudgetProgramID     = "ComputeBudget111111111111111111111111111111"
	programDerivedAddressLabel = "ProgramDerivedAddress"
)

// ErrInvalidSeeds is returned when PDA seeds exceed Solana's limits.
var ErrInvalidSeeds = errors.New("invalid PDA seeds")

var (
	tokenProgramID           = mustPublicKey(TokenProgramID)
	associatedTokenProgramID = mustPublicKey(AssociatedTokenProgramID)
)

// DeriveAssociatedTokenAddress returns the associated token account for an
// owner and mint using the standard seeds [owner, tokenProgram, mint].
func DeriveAssociatedTokenAddress(owner, mint string) (string, error) {
	ownerBytes, err := DecodeAddress(owner)
	if err != nil {
		return "", fmt.Errorf("owner: %w", err)
	}
	mintBytes, err := DecodeAddress(mint)
	if err != nil {
		return "", fmt.Errorf("mint: %w", err)
	}
	address, _, err := findProgramAddress(
		[][]byte{ownerBytes, tokenProgramID[:], mintBytes},
		associatedTokenProgramID[:],
	)
	if err != nil {
		return "", err
	}
	return base58.Encode(address), nil
}

func findProgramAddress(seeds [][]byte, program []byte) ([]byte, byte, error) {
	if len(seeds) == 0 || len(seeds) > 16 {
		return nil, 0, fmt.Errorf("%w: seed count must be 1-16", ErrInvalidSeeds)
	}
	total := 0
	for _, seed := range seeds {
		if len(seed) > PublicKeyLength {
			return nil, 0, fmt.Errorf("%w: seed exceeds %d bytes", ErrInvalidSeeds, PublicKeyLength)
		}
		total += len(seed)
	}
	if len(program) != PublicKeyLength {
		return nil, 0, fmt.Errorf("%w: program must be %d bytes", ErrInvalidSeeds, PublicKeyLength)
	}
	if total+len(seeds)+len(program)+len(programDerivedAddressLabel) > 1024 {
		return nil, 0, fmt.Errorf("%w: PDA preimage too large", ErrInvalidSeeds)
	}

	hash := sha256.New()
	for bump := 255; bump >= 0; bump-- {
		hash.Reset()
		for _, seed := range seeds {
			_, _ = hash.Write(seed)
		}
		_, _ = hash.Write([]byte{byte(bump)})
		_, _ = hash.Write(program)
		_, _ = hash.Write([]byte(programDerivedAddressLabel))
		digest := hash.Sum(nil)
		if !isOnCurve(digest) {
			return digest, byte(bump), nil
		}
	}
	return nil, 0, errors.New("solana: no viable PDA bump seed")
}

func isOnCurve(candidate []byte) bool {
	var point edwards25519.Point
	_, err := point.SetBytes(candidate)
	return err == nil
}

func mustPublicKey(value string) []byte {
	raw, err := DecodeAddress(value)
	if err != nil {
		panic("solana: invalid built-in public key: " + err.Error())
	}
	return raw
}
