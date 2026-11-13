package evm

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// ERC20TransferSelector is the 4-byte selector for transfer(address,uint256).
var ERC20TransferSelector = [4]byte{0xa9, 0x05, 0x9c, 0xbb}

// ErrInvalidCallData is returned for malformed ERC-20 calldata.
var ErrInvalidCallData = errors.New("invalid ERC-20 calldata")

// ERC20TransferData builds standard transfer(address,uint256) calldata.
func ERC20TransferData(recipient string, amount *big.Int) ([]byte, error) {
	address, err := DecodeAddress(recipient)
	if err != nil {
		return nil, err
	}
	if amount == nil || amount.Sign() < 0 || amount.BitLen() > 256 {
		return nil, fmt.Errorf("%w: amount must be an unsigned 256-bit integer", ErrInvalidCallData)
	}
	data := make([]byte, 4+32+32)
	copy(data[:4], ERC20TransferSelector[:])
	copy(data[4+12:4+32], address)
	amount.FillBytes(data[4+32 : 4+64])
	return data, nil
}

// DecodeERC20Transfer parses standard transfer(address,uint256) calldata.
func DecodeERC20Transfer(data []byte) (string, *big.Int, error) {
	if len(data) != 4+32+32 {
		return "", nil, fmt.Errorf("%w: expected 68 bytes, got %d", ErrInvalidCallData, len(data))
	}
	if [4]byte(data[:4]) != ERC20TransferSelector {
		return "", nil, fmt.Errorf("%w: wrong selector 0x%s", ErrInvalidCallData, hex.EncodeToString(data[:4]))
	}
	for _, b := range data[4:16] {
		if b != 0 {
			return "", nil, fmt.Errorf("%w: address padding is not zero", ErrInvalidCallData)
		}
	}
	address, err := Checksum(data[16:36])
	if err != nil {
		return "", nil, err
	}
	amount := new(big.Int).SetBytes(data[36:68])
	return address, amount, nil
}

// ERC20TransferDataHex returns 0x-prefixed calldata for JSON-RPC payloads.
func ERC20TransferDataHex(recipient string, amount *big.Int) (string, error) {
	data, err := ERC20TransferData(recipient, amount)
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(data), nil
}

// NormalizeCallData validates 0x-prefixed hex calldata and returns lowercase.
func NormalizeCallData(value string) (string, error) {
	if !strings.HasPrefix(value, "0x") {
		return "", fmt.Errorf("%w: calldata must start with 0x", ErrInvalidCallData)
	}
	body := value[2:]
	if len(body)%2 != 0 {
		return "", fmt.Errorf("%w: odd-length hex", ErrInvalidCallData)
	}
	if _, err := hex.DecodeString(body); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidCallData, err)
	}
	return "0x" + strings.ToLower(body), nil
}
