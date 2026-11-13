package wallets

import (
	"encoding/hex"
	"fmt"
	"strconv"

	"universal-defi-intelligence/internal/chains/stellar"
)

// StellarTransferRequest describes an unsigned Stellar payment.
type StellarTransferRequest struct {
	SourceAccount string
	Sequence      int64
	BaseFee       uint32
	TimeBounds    *StellarTimeBounds
	Destination   string
	Asset         StellarAsset
	Amount        int64
	Memo          stellar.Memo
}

// StellarAsset identifies native XLM (empty Code) or an issued asset.
type StellarAsset struct {
	Code   string
	Issuer string
}

// StellarTimeBounds maps to Stellar's optional TimeBounds.
type StellarTimeBounds struct {
	MinTime uint64
	MaxTime uint64
}

// StellarMemo is a JSON-friendly memo summary. The XDR remains authoritative.
type StellarMemo struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	ID   string `json:"id,omitempty"`
	Hash string `json:"hash,omitempty"`
}

// StellarPayload is a base64 TransactionEnvelope XDR with no signatures.
type StellarPayload struct {
	XDR           string      `json:"xdr"`
	Memo          StellarMemo `json:"memo"`
	SourceAccount string      `json:"sourceAccount"`
	Sequence      string      `json:"sequence"`
}

// BuildStellarTransfer returns an unsigned Stellar V1 transaction envelope.
func BuildStellarTransfer(req StellarTransferRequest) (StellarPayload, error) {
	if req.Sequence < 0 {
		return StellarPayload{}, fmt.Errorf("wallets: sequence must not be negative")
	}
	if req.BaseFee == 0 {
		return StellarPayload{}, fmt.Errorf("wallets: base fee must be greater than zero")
	}
	if req.Amount <= 0 {
		return StellarPayload{}, fmt.Errorf("%w: amount must be greater than zero", ErrInvalidAmount)
	}
	if req.TimeBounds != nil && req.TimeBounds.MaxTime != 0 && req.TimeBounds.MinTime > req.TimeBounds.MaxTime {
		return StellarPayload{}, fmt.Errorf("wallets: time bounds min exceeds max")
	}
	source, err := stellar.ParseAccountID(req.SourceAccount)
	if err != nil {
		return StellarPayload{}, fmt.Errorf("source account: %w", err)
	}
	destination, err := stellar.ParseAccountID(req.Destination)
	if err != nil {
		return StellarPayload{}, fmt.Errorf("destination: %w", err)
	}
	if err := req.Memo.Validate(); err != nil {
		return StellarPayload{}, err
	}
	assetType, code, issuer, err := stellarAsset(req.Asset)
	if err != nil {
		return StellarPayload{}, err
	}

	writer := &xdrWriter{}
	writer.int32(2) // ENVELOPE_TYPE_TX
	writeMuxedAccount(writer, source)
	writer.uint32(req.BaseFee)
	writer.int64(req.Sequence)
	writer.bool(req.TimeBounds != nil)
	if req.TimeBounds != nil {
		writer.uint64(req.TimeBounds.MinTime)
		writer.uint64(req.TimeBounds.MaxTime)
	}
	writeMemo(writer, req.Memo)
	writer.uint32(1)   // operations
	writer.bool(false) // no operation-level source account
	writer.int32(1)    // PAYMENT
	writeMuxedAccount(writer, destination)
	writer.int32(assetType)
	if assetType != 0 {
		writer.fixedOpaque(code)
		writeMuxedAccount(writer, issuer)
	}
	writer.int64(req.Amount)
	writer.int32(0)  // ext
	writer.uint32(0) // signatures

	memo := StellarMemo{Type: string(req.Memo.Type)}
	if memo.Type == "" {
		memo.Type = string(stellar.MemoNone)
	}
	switch req.Memo.Type {
	case stellar.MemoText:
		memo.Text = req.Memo.Text
	case stellar.MemoID:
		memo.ID = strconv.FormatUint(req.Memo.ID, 10)
	case stellar.MemoHash, stellar.MemoReturn:
		memo.Hash = hex.EncodeToString(req.Memo.Hash)
	}
	return StellarPayload{
		XDR:           writer.base64(),
		Memo:          memo,
		SourceAccount: source,
		Sequence:      strconv.FormatInt(req.Sequence, 10),
	}, nil
}

func stellarAsset(asset StellarAsset) (int32, []byte, string, error) {
	if asset.Code == "" {
		if asset.Issuer != "" {
			return 0, nil, "", fmt.Errorf("wallets: native asset must not have an issuer")
		}
		return 0, nil, "", nil
	}
	if len(asset.Code) > 12 {
		return 0, nil, "", fmt.Errorf("wallets: asset code must be 1-12 characters")
	}
	for _, char := range asset.Code {
		if !((char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9')) {
			return 0, nil, "", fmt.Errorf("wallets: asset code must be alphanumeric")
		}
	}
	if _, err := stellar.ParseAccountID(asset.Issuer); err != nil {
		return 0, nil, "", fmt.Errorf("asset issuer: %w", err)
	}
	assetType := int32(1)
	width := 4
	if len(asset.Code) > 4 {
		assetType = 2
		width = 12
	}
	code := make([]byte, width)
	copy(code, asset.Code)
	return assetType, code, asset.Issuer, nil
}

func writeMuxedAccount(writer *xdrWriter, address string) {
	raw, _ := stellar.DecodeAccountID(address)
	writer.int32(0) // KEY_TYPE_ED25519
	writer.fixedOpaque(raw)
}

func writeMemo(writer *xdrWriter, memo stellar.Memo) {
	switch memo.Type {
	case "", stellar.MemoNone:
		writer.int32(0)
	case stellar.MemoText:
		writer.int32(1)
		writer.varOpaque([]byte(memo.Text))
	case stellar.MemoID:
		writer.int32(2)
		writer.uint64(memo.ID)
	case stellar.MemoHash:
		writer.int32(3)
		writer.fixedOpaque(memo.Hash)
	case stellar.MemoReturn:
		writer.int32(4)
		writer.fixedOpaque(memo.Hash)
	}
}
