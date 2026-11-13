package nearoneclick

import (
	"context"
	"net/http"
	"strings"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

type submitDepositRequest struct {
	TxHash         string `json:"txHash"`
	DepositAddress string `json:"depositAddress"`
	Memo           string `json:"memo,omitempty"`
}

// SubmitDepositTx notifies 1Click about a source transaction. It is an
// accelerator, not the source of truth: callers must record the transaction
// before calling and must never blind-retry, because the provider treats the
// call as idempotent only when the (txHash, depositAddress) pair is unchanged.
func (c *Client) SubmitDepositTx(ctx context.Context, req providers.DepositRequest) (providers.StatusResult, error) {
	txHash := strings.TrimSpace(req.TxHash)
	if txHash == "" || len(txHash) > maxTxHashLength {
		return providers.StatusResult{}, domain.New(domain.CodeInvalidTxHash,
			"A valid source transaction hash is required.")
	}
	address := strings.TrimSpace(req.DepositAddress)
	if address == "" || len(address) > maxAddressLength {
		return providers.StatusResult{}, domain.New(domain.CodeInvalidDeposit,
			"A valid provider deposit address is required.")
	}

	payload := submitDepositRequest{
		TxHash:         txHash,
		DepositAddress: address,
		Memo:           strings.TrimSpace(req.DepositMemo),
	}
	body, _, err := c.request(ctx, http.MethodPost, "/v0/deposit/submit", payload, true)
	if err != nil {
		return providers.StatusResult{}, err
	}
	return normalizeStatus(body, c.now())
}
