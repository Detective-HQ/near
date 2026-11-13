package wallets

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"universal-defi-intelligence/internal/chains/solana"
)

// MaxSolanaTransactionBytes is the packet limit for a serialized message.
const MaxSolanaTransactionBytes = 1232

// SolanaTransferRequest describes an unsigned SOL or SPL token transfer.
// For SPL transfers, To must be the destination token account.
type SolanaTransferRequest struct {
	From            string
	To              string
	Mint            string
	Memo            string
	RecentBlockhash string
	Amount          uint64
}

// SolanaPayload is the base64 legacy message plus the memo instruction value.
type SolanaPayload struct {
	Message         string `json:"message"`
	Memo            string `json:"memo,omitempty"`
	RecentBlockhash string `json:"recentBlockhash"`
}

type solanaInstruction struct {
	program  string
	accounts []string
	data     []byte
}

type solanaAccount struct {
	key      string
	signer   bool
	writable bool
}

// BuildSolanaTransfer compiles an unsigned legacy Solana message containing
// one transfer instruction and, when present, a memo instruction.
func BuildSolanaTransfer(req SolanaTransferRequest) (SolanaPayload, error) {
	if req.Amount == 0 {
		return SolanaPayload{}, fmt.Errorf("%w: must be greater than zero", ErrInvalidAmount)
	}
	if len(req.Memo) > MaxMemoBytes {
		return SolanaPayload{}, fmt.Errorf("wallets: memo exceeds %d bytes", MaxMemoBytes)
	}
	from, err := solana.ParseAddress(req.From)
	if err != nil {
		return SolanaPayload{}, fmt.Errorf("from: %w", err)
	}
	to, err := solana.ParseAddress(req.To)
	if err != nil {
		return SolanaPayload{}, fmt.Errorf("to: %w", err)
	}
	blockhash, err := solana.ParseAddress(req.RecentBlockhash)
	if err != nil {
		return SolanaPayload{}, fmt.Errorf("recent blockhash: %w", err)
	}

	var instructions []solanaInstruction
	if req.Mint == "" {
		data := make([]byte, 12)
		binary.LittleEndian.PutUint32(data[:4], 2)
		binary.LittleEndian.PutUint64(data[4:], req.Amount)
		instructions = append(instructions, solanaInstruction{
			program:  solana.SystemProgramID,
			accounts: []string{from, to},
			data:     data,
		})
	} else {
		mint, err := solana.ParseAddress(req.Mint)
		if err != nil {
			return SolanaPayload{}, fmt.Errorf("mint: %w", err)
		}
		source, err := solana.DeriveAssociatedTokenAddress(from, mint)
		if err != nil {
			return SolanaPayload{}, fmt.Errorf("source token account: %w", err)
		}
		data := make([]byte, 9)
		data[0] = 3
		binary.LittleEndian.PutUint64(data[1:], req.Amount)
		instructions = append(instructions, solanaInstruction{
			program:  solana.TokenProgramID,
			accounts: []string{source, to, from},
			data:     data,
		})
	}
	if req.Memo != "" {
		instructions = append(instructions, solanaInstruction{
			program:  solana.MemoProgramID,
			accounts: []string{from},
			data:     []byte(req.Memo),
		})
	}

	accounts := []solanaAccount{{key: from, signer: true, writable: true}}
	addAccount := func(key string, writable bool) int {
		for i := range accounts {
			if accounts[i].key == key {
				accounts[i].writable = accounts[i].writable || writable
				return i
			}
		}
		accounts = append(accounts, solanaAccount{key: key, writable: writable})
		return len(accounts) - 1
	}

	for _, instruction := range instructions {
		for _, account := range instruction.accounts {
			addAccount(account, true)
		}
	}
	for _, instruction := range instructions {
		addAccount(instruction.program, false)
	}
	sort.SliceStable(accounts, func(i, j int) bool {
		left, right := accounts[i], accounts[j]
		if left.signer != right.signer {
			return left.signer
		}
		if left.writable != right.writable {
			return left.writable
		}
		return left.key < right.key
	})
	accountIndex := make(map[string]int, len(accounts))
	for i, account := range accounts {
		accountIndex[account.key] = i
	}

	numRequired := 0
	numReadonlySigned := 0
	numReadonlyUnsigned := 0
	for _, account := range accounts {
		switch {
		case account.signer:
			numRequired++
			if !account.writable {
				numReadonlySigned++
			}
		case !account.writable:
			numReadonlyUnsigned++
		}
	}

	var message []byte
	message = append(message, byte(numRequired), byte(numReadonlySigned), byte(numReadonlyUnsigned))
	message = appendCompactU16(message, len(accounts))
	for _, account := range accounts {
		raw, err := solana.DecodeAddress(account.key)
		if err != nil {
			return SolanaPayload{}, err
		}
		message = append(message, raw...)
	}
	blockhashBytes, _ := solana.DecodeAddress(blockhash)
	message = append(message, blockhashBytes...)
	message = appendCompactU16(message, len(instructions))
	for _, instruction := range instructions {
		message = append(message, byte(accountIndex[instruction.program]))
		message = appendCompactU16(message, len(instruction.accounts))
		for _, account := range instruction.accounts {
			message = append(message, byte(accountIndex[account]))
		}
		message = appendCompactU16(message, len(instruction.data))
		message = append(message, instruction.data...)
	}
	if len(message) > MaxSolanaTransactionBytes {
		return SolanaPayload{}, errors.New("wallets: Solana message exceeds transaction size limit")
	}
	return SolanaPayload{
		Message:         base64.StdEncoding.EncodeToString(message),
		Memo:            req.Memo,
		RecentBlockhash: blockhash,
	}, nil
}

func appendCompactU16(dst []byte, value int) []byte {
	for {
		byteValue := byte(value & 0x7f)
		value >>= 7
		if value == 0 {
			return append(dst, byteValue)
		}
		dst = append(dst, byteValue|0x80)
	}
}
