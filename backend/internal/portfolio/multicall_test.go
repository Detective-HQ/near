package portfolio

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
)

func TestMulticall3EncodingAndDecoding(t *testing.T) {
	tokens := []TokenAsset{
		{
			ID:              "base:usdc",
			Symbol:          "USDC",
			Chain:           "base",
			Decimals:        6,
			ContractAddress: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
		},
		{
			ID:              "base:weth",
			Symbol:          "WETH",
			Chain:           "base",
			Decimals:        18,
			ContractAddress: "0x4200000000000000000000000000000000000006",
		},
	}
	owner := "0x1111111111111111111111111111111111111111"

	encoded, err := encodeMulticall3BalanceOf(tokens, owner)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	if !strings.HasPrefix(encoded, "0x82ad56cb") {
		t.Fatalf("expected selector 0x82ad56cb, got %s", encoded[:10])
	}

	// Mock aggregate3 response: 2 results:
	// result 0: success = true, balance = 500_000000 (500 USDC)
	// result 1: success = true, balance = 0
	// Layout:
	// 0..32: offset to Result[] array = 32 (0x20)
	// 32..64: array len = 2
	// 64..96: head[0] offset from byte 32 = 96 (0x60) -> byte 128
	// 96..128: head[1] offset from byte 32 = 224 (0xe0) -> byte 256
	// 128..256: Result 0 (128 bytes)
	// 256..384: Result 1 (128 bytes)
	resBuf := make([]byte, 384)
	resBuf[31] = 0x20 // offset to array
	resBuf[63] = 0x02 // array len = 2
	resBuf[95] = 0x60 // head[0] offset = 96 -> pos 128
	resBuf[127] = 0xe0 // head[1] offset = 224 -> pos 256

	// Result 0 at byte 128
	r0 := 128
	resBuf[r0+31] = 0x01 // success = true
	resBuf[r0+63] = 0x40 // data offset within struct = 64
	resBuf[r0+95] = 0x20 // data length = 32
	big.NewInt(500000000).FillBytes(resBuf[r0+96 : r0+128])

	// Result 1 at byte 256
	r1 := 256
	resBuf[r1+31] = 0x01 // success = true
	resBuf[r1+63] = 0x40 // data offset within struct = 64
	resBuf[r1+95] = 0x20 // data length = 32
	big.NewInt(0).FillBytes(resBuf[r1+96 : r1+128])

	mockHex := "0x" + hex.EncodeToString(resBuf)
	decoded, err := decodeMulticall3Results(mockHex, len(tokens))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(decoded) != 2 {
		t.Fatalf("expected 2 decoded results, got %d", len(decoded))
	}
	if decoded[0] == nil || decoded[0].Cmp(big.NewInt(500000000)) != 0 {
		t.Fatalf("expected 500000000, got %v", decoded[0])
	}
	if decoded[1] == nil || decoded[1].Sign() != 0 {
		t.Fatalf("expected 0, got %v", decoded[1])
	}
}
