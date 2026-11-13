package service

import (
	"encoding/hex"
	"fmt"
	"strings"

	"universal-defi-intelligence/internal/chains/aptos"
	"universal-defi-intelligence/internal/chains/evm"
	"universal-defi-intelligence/internal/chains/near"
	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/chains/solana"
	"universal-defi-intelligence/internal/chains/stellar"
	"universal-defi-intelligence/internal/encoding/base58"
	"universal-defi-intelligence/internal/swap/domain"
)

// ValidateChainAddress validates an address against the rules of the chain's family.
func ValidateChainAddress(reg *registry.Registry, chainKey, address string) (string, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", domain.New(domain.CodeValidationFailed, "Address is required.")
	}

	chain, found := reg.Get(chainKey)
	if !found {
		return "", domain.Newf(domain.CodeUnknownAsset, "Chain %q is not in the registry.", chainKey)
	}

	switch chain.Family {
	case registry.FamilyEVM:
		parsed, err := evm.ParseAddress(address)
		if err != nil {
			return "", domain.Wrap(domain.CodeInvalidRecipient, "Invalid EVM address.", err)
		}
		return parsed, nil

	case registry.FamilySolana:
		parsed, err := solana.ParseAddress(address)
		if err != nil {
			return "", domain.Wrap(domain.CodeInvalidRecipient, "Invalid Solana address.", err)
		}
		return parsed, nil

	case registry.FamilyStellar:
		parsed, err := stellar.ParseAccountID(address)
		if err != nil {
			return "", domain.Wrap(domain.CodeInvalidRecipient, "Invalid Stellar account ID.", err)
		}
		return parsed, nil

	case registry.FamilyMove:
		parsed, err := aptos.ParseAddress(address)
		if err != nil {
			return "", domain.Wrap(domain.CodeInvalidRecipient, "Invalid Aptos address.", err)
		}
		return parsed, nil

	case registry.FamilyNEAR:
		parsed, err := near.ParseAccountID(address)
		if err != nil {
			return "", domain.Wrap(domain.CodeInvalidRecipient, "Invalid NEAR account ID.", err)
		}
		return parsed, nil

	default:
		return "", domain.Newf(domain.CodeValidationFailed, "Unsupported chain family %q.", chain.Family)
	}
}

// ValidateTxHash validates a transaction hash format per origin chain family.
func ValidateTxHash(reg *registry.Registry, chainKey, txHash string) (string, error) {
	txHash = strings.TrimSpace(txHash)
	if txHash == "" {
		return "", domain.New(domain.CodeInvalidTxHash, "Transaction hash is required.")
	}

	chain, found := reg.Get(chainKey)
	if !found {
		return "", domain.Newf(domain.CodeUnknownAsset, "Chain %q is not in the registry.", chainKey)
	}

	switch chain.Family {
	case registry.FamilyEVM:
		parsed, err := evm.ParseTxHash(txHash)
		if err != nil {
			return "", domain.Wrap(domain.CodeInvalidTxHash, "Invalid EVM transaction hash.", err)
		}
		return parsed, nil

	case registry.FamilySolana:
		// Solana transaction signatures are base58-encoded 64-byte Ed25519 signatures.
		raw, err := base58.Decode(txHash)
		if err != nil || len(raw) != 64 {
			return "", domain.New(domain.CodeInvalidTxHash, "Invalid Solana transaction signature.")
		}
		return txHash, nil

	case registry.FamilyStellar:
		// Stellar transaction hashes are 32 bytes (64 hex characters).
		clean := strings.TrimPrefix(strings.ToLower(txHash), "0x")
		if len(clean) != 64 {
			return "", domain.New(domain.CodeInvalidTxHash, "Invalid Stellar transaction hash length.")
		}
		if _, err := hex.DecodeString(clean); err != nil {
			return "", domain.New(domain.CodeInvalidTxHash, "Invalid Stellar transaction hash characters.")
		}
		return clean, nil

	case registry.FamilyMove:
		// Aptos transaction hashes are 32 bytes (64 hex characters with optional 0x).
		clean := strings.TrimPrefix(strings.ToLower(txHash), "0x")
		if len(clean) != 64 {
			return "", domain.New(domain.CodeInvalidTxHash, "Invalid Aptos transaction hash length.")
		}
		if _, err := hex.DecodeString(clean); err != nil {
			return "", domain.New(domain.CodeInvalidTxHash, "Invalid Aptos transaction hash characters.")
		}
		return "0x" + clean, nil

	case registry.FamilyNEAR:
		// NEAR transaction hashes are base58-encoded 32-byte SHA-256 hashes.
		raw, err := base58.Decode(txHash)
		if err != nil || len(raw) != 32 {
			return "", domain.New(domain.CodeInvalidTxHash, "Invalid NEAR transaction hash.")
		}
		return txHash, nil

	default:
		return "", domain.Newf(domain.CodeInvalidTxHash, "Unsupported chain family %q.", chain.Family)
	}
}

// ValidateStellarMemo ensures a memo complies with Stellar byte limits when required.
func ValidateStellarMemo(memoStr string) error {
	if strings.TrimSpace(memoStr) == "" {
		return nil
	}
	memo := stellar.Memo{
		Type: stellar.MemoText,
		Text: memoStr,
	}
	if err := memo.Validate(); err != nil {
		return domain.Wrap(domain.CodeValidationFailed, fmt.Sprintf("Invalid Stellar memo: %v", err), err)
	}
	return nil
}

