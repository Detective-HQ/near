package wallets

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"universal-defi-intelligence/internal/chains/stellar"
)

func TestBuildEVMTransfer(t *testing.T) {
	t.Parallel()
	native, err := BuildEVMTransfer(EVMTransferRequest{
		ChainID: "0x1",
		To:      "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed",
		Amount:  "1000000000000000000",
	})
	if err != nil {
		t.Fatalf("BuildEVMTransfer(native): %v", err)
	}
	want := EVMPayload{
		To:      "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		Value:   "1000000000000000000",
		Data:    "0x",
		ChainID: "1",
	}
	if native != want {
		t.Fatalf("native = %+v, want %+v", native, want)
	}

	token := "0xdAC17F958D2ee523a2206206994597C13D831ec7"
	erc20, err := BuildEVMTransfer(EVMTransferRequest{
		ChainID: "1",
		To:      "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed",
		Token:   token,
		Amount:  "1000000",
	})
	if err != nil {
		t.Fatalf("BuildEVMTransfer(erc20): %v", err)
	}
	if erc20.To != token || erc20.Value != "0" || !strings.HasPrefix(erc20.Data, "0xa9059cbb") || len(erc20.Data) != 138 {
		t.Fatalf("erc20 = %+v", erc20)
	}
}

func TestBuildEVMTransferRejectsInvalid(t *testing.T) {
	t.Parallel()
	base := EVMTransferRequest{ChainID: "1", To: "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed", Amount: "1"}
	cases := []EVMTransferRequest{
		{},
		{ChainID: "0x0", To: base.To, Amount: "1"},
		{ChainID: "1", To: "bad", Amount: "1"},
		{ChainID: "1", To: base.To, Amount: "0"},
		{ChainID: "1", To: base.To, Amount: "1.5"},
		{ChainID: "1", To: base.To, Amount: "1", Memo: "memo"},
		{ChainID: "1", To: base.To, Token: "bad", Amount: "1"},
		{ChainID: "1", To: base.To, Amount: strings.Repeat("9", 100)},
	}
	for _, req := range cases {
		if _, err := BuildEVMTransfer(req); err == nil {
			t.Fatalf("BuildEVMTransfer(%+v) succeeded", req)
		}
	}
}

func TestBuildAptosTransfer(t *testing.T) {
	t.Parallel()
	native, err := BuildAptosTransfer(AptosTransferRequest{
		Recipient: "0x1",
		Amount:    "1000",
	})
	if err != nil {
		t.Fatalf("BuildAptosTransfer(native): %v", err)
	}
	if native.Function != "0x1::aptos_account::transfer" || len(native.TypeArguments) != 0 || native.Arguments[0] != "0x"+strings.Repeat("0", 63)+"1" {
		t.Fatalf("native = %+v", native)
	}
	coinType := "0x1::aptos_coin::AptosCoin"
	coin, err := BuildAptosTransfer(AptosTransferRequest{Recipient: "0x1", Amount: "5", CoinType: coinType})
	if err != nil {
		t.Fatalf("BuildAptosTransfer(coin): %v", err)
	}
	wantType := "0x" + strings.Repeat("0", 63) + "1::aptos_coin::AptosCoin"
	if coin.Function != "0x1::coin::transfer" || len(coin.TypeArguments) != 1 || coin.TypeArguments[0] != wantType {
		t.Fatalf("coin = %+v", coin)
	}
	if _, err := BuildAptosTransfer(AptosTransferRequest{Recipient: "0x1", Amount: "5", CoinType: "u64"}); err == nil {
		t.Fatal("primitive coin type accepted")
	}
	if _, err := BuildAptosTransfer(AptosTransferRequest{Recipient: "nothex", Amount: "5"}); err == nil {
		t.Fatal("bad recipient accepted")
	}
}

func TestBuildNEARTransfer(t *testing.T) {
	t.Parallel()
	native, err := BuildNEARTransfer(NEARTransferRequest{DepositAddress: "alice.near", Amount: "1000"})
	if err != nil {
		t.Fatalf("BuildNEARTransfer(native): %v", err)
	}
	if native.ReceiverID != "alice.near" || len(native.Actions) != 1 || native.Actions[0].Type != "Transfer" || native.Actions[0].Params["deposit"] != "1000" {
		t.Fatalf("native = %+v", native)
	}

	ft, err := BuildNEARTransfer(NEARTransferRequest{
		DepositAddress: "alice.near",
		Amount:         "42",
		TokenContract:  "usdc.near",
		Memo:           "deposit-42",
	})
	if err != nil {
		t.Fatalf("BuildNEARTransfer(ft): %v", err)
	}
	if ft.ReceiverID != "usdc.near" || ft.Actions[0].Type != "FunctionCall" {
		t.Fatalf("ft = %+v", ft)
	}
	encoded := ft.Actions[0].Params["args"]
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode args: %v", err)
	}
	var args map[string]string
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatalf("unmarshal args: %v", err)
	}
	if args["receiver_id"] != "alice.near" || args["amount"] != "42" || args["memo"] != "deposit-42" {
		t.Fatalf("args = %+v", args)
	}
	if ft.Actions[0].Params["gas"] != "30000000000000" || ft.Actions[0].Params["deposit"] != "1" {
		t.Fatalf("gas/deposit = %+v", ft.Actions[0].Params)
	}
	if _, err := BuildNEARTransfer(NEARTransferRequest{DepositAddress: "alice.near", Amount: "1", Memo: "x"}); err == nil {
		t.Fatal("native NEAR memo accepted")
	}
	if _, err := BuildNEARTransfer(NEARTransferRequest{DepositAddress: "Alice.near", Amount: "1"}); err == nil {
		t.Fatal("bad account accepted")
	}
	if _, err := BuildNEARTransfer(NEARTransferRequest{DepositAddress: "alice.near", Amount: "1", TokenContract: "usdc.near", Memo: strings.Repeat("a", MaxMemoBytes+1)}); err == nil {
		t.Fatal("oversized memo accepted")
	}
}

func TestBuildSolanaTransferVectors(t *testing.T) {
	t.Parallel()
	native, err := BuildSolanaTransfer(SolanaTransferRequest{
		From:            "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin",
		To:              "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU",
		Memo:            "hello",
		RecentBlockhash: "11111111111111111111111111111111",
		Amount:          123456789,
	})
	if err != nil {
		t.Fatalf("BuildSolanaTransfer(native): %v", err)
	}
	wantNative := "AQACBIUPLW4CpHr4JNCatp3ELXDLKMv6JJ+37le50lbBJ2LvZ1IFXCCz6dh0Zlbd9zhVUH+Hq22HUj5Mdqf6NglqmesAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAVKU1qZKSEGTSTocWDaOHx8NbXdvJK7geQfqEBBBUSNAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAgIAAQwCAAAAFc1bBwAAAAADAQAFaGVsbG8="
	if native.Message != wantNative {
		t.Fatalf("native message = %s, want %s", native.Message, wantNative)
	}

	spl, err := BuildSolanaTransfer(SolanaTransferRequest{
		From:            "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin",
		To:              "Gvdu5vziankrzZkxPecKtxpH881iRMVKAsNqocJYyEtF",
		Mint:            "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		Memo:            "deposit-123",
		RecentBlockhash: "11111111111111111111111111111111",
		Amount:          1000000,
	})
	if err != nil {
		t.Fatalf("BuildSolanaTransfer(spl): %v", err)
	}
	wantSPL := "AQACBYUPLW4CpHr4JNCatp3ELXDLKMv6JJ+37le50lbBJ2Lv0OuWd601oec7WdvAjXPQPd9+X1CaqcrPLQ7XWmtBUDrsnM64KxQ9+e3KvjhF+voQlegPteJSKFoesv17mKITgAVKU1qZKSEGTSTocWDaOHx8NbXdvJK7geQfqEBBBUSNBt324ddloZPZy+FGzut5rBy0he1fWzeROoz1hX7/AKkAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAIEAwECAAkDQEIPAAAAAAADAQALZGVwb3NpdC0xMjM="
	if spl.Message != wantSPL {
		t.Fatalf("spl message = %s, want %s", spl.Message, wantSPL)
	}
}

func TestBuildSolanaTransferRejectsInvalid(t *testing.T) {
	t.Parallel()
	base := SolanaTransferRequest{
		From:            "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin",
		To:              "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU",
		RecentBlockhash: "11111111111111111111111111111111",
		Amount:          1,
	}
	cases := []SolanaTransferRequest{
		{},
		{From: "bad", To: base.To, RecentBlockhash: base.RecentBlockhash, Amount: 1},
		{From: base.From, To: "bad", RecentBlockhash: base.RecentBlockhash, Amount: 1},
		{From: base.From, To: base.To, RecentBlockhash: "bad", Amount: 1},
		{From: base.From, To: base.To, RecentBlockhash: base.RecentBlockhash, Amount: 0},
		{From: base.From, To: base.To, RecentBlockhash: base.RecentBlockhash, Amount: 1, Mint: "bad"},
		{From: base.From, To: base.To, RecentBlockhash: base.RecentBlockhash, Amount: 1, Memo: strings.Repeat("a", MaxMemoBytes+1)},
	}
	for _, req := range cases {
		if _, err := BuildSolanaTransfer(req); err == nil {
			t.Fatalf("BuildSolanaTransfer(%+v) succeeded", req)
		}
	}
}

func TestBuildStellarTransferVectors(t *testing.T) {
	t.Parallel()
	base := StellarTransferRequest{
		SourceAccount: "GBGLLK7WVV47X5NLXTFPZQTJ3BONEZI62S4ILNMGT4SBV3PQUW5CTECA",
		Sequence:      12346,
		BaseFee:       100,
		TimeBounds:    &StellarTimeBounds{MinTime: 0, MaxTime: 1893456000},
		Destination:   "GB2CFOMIOWMANDRSYRCIVFE23MUQ2D2OGW46AGYO4XY2DZQA7YTHIJGX",
		Memo:          stellar.Memo{Type: stellar.MemoText, Text: "hello"},
		Amount:        12345678,
	}
	native, err := BuildStellarTransfer(base)
	if err != nil {
		t.Fatalf("BuildStellarTransfer(native): %v", err)
	}
	wantNative := "AAAAAgAAAABMtav2rXn79au8yvzCadhc0mUe1LiFtYafJBrt8KW6KQAAAGQAAAAAAAAwOgAAAAEAAAAAAAAAAAAAAABw29iAAAAAAQAAAAVoZWxsbwAAAAAAAAEAAAAAAAAAAQAAAAB0IrmIdZgGjjLERIqUmtspDQ9ONbngGw7l8aHmAP4mdAAAAAAAAAAAALxhTgAAAAAAAAAA"
	if native.XDR != wantNative {
		t.Fatalf("native XDR = %s, want %s", native.XDR, wantNative)
	}
	if native.Memo.Type != "text" || native.Memo.Text != "hello" || native.Sequence != "12346" {
		t.Fatalf("native payload = %+v", native)
	}

	issued := base
	issued.Asset = StellarAsset{Code: "USDC", Issuer: "GBLJTKOO7BYOF7YMAIVWO2E4Y5X6AXUQSFOF6AKD7E2WZJZPJL7ZTETK"}
	issued.Memo = stellar.Memo{Type: stellar.MemoID, ID: 42}
	issued.Amount = 25000000
	usdc, err := BuildStellarTransfer(issued)
	if err != nil {
		t.Fatalf("BuildStellarTransfer(usdc): %v", err)
	}
	wantUSDC := "AAAAAgAAAABMtav2rXn79au8yvzCadhc0mUe1LiFtYafJBrt8KW6KQAAAGQAAAAAAAAwOgAAAAEAAAAAAAAAAAAAAABw29iAAAAAAgAAAAAAAAAqAAAAAQAAAAAAAAABAAAAAHQiuYh1mAaOMsREipSa2ykND041ueAbDuXxoeYA/iZ0AAAAAVVTREMAAAAAVpmpzvhw4v8MAitnaJzHb+BekJFcXwFD+TVspy9K/5kAAAAAAX14QAAAAAAAAAAA"
	if usdc.XDR != wantUSDC {
		t.Fatalf("usdc XDR = %s, want %s", usdc.XDR, wantUSDC)
	}
	if usdc.Memo.ID != "42" {
		t.Fatalf("usdc memo = %+v", usdc.Memo)
	}
}

func TestBuildStellarTransferRejectsInvalid(t *testing.T) {
	t.Parallel()
	valid := StellarTransferRequest{
		SourceAccount: "GBGLLK7WVV47X5NLXTFPZQTJ3BONEZI62S4ILNMGT4SBV3PQUW5CTECA",
		Sequence:      1,
		BaseFee:       100,
		Destination:   "GB2CFOMIOWMANDRSYRCIVFE23MUQ2D2OGW46AGYO4XY2DZQA7YTHIJGX",
		Amount:        1,
	}
	cases := []StellarTransferRequest{}
	req := valid
	req.Sequence = -1
	cases = append(cases, req)
	req = valid
	req.BaseFee = 0
	cases = append(cases, req)
	req = valid
	req.Amount = 0
	cases = append(cases, req)
	req = valid
	req.TimeBounds = &StellarTimeBounds{MinTime: 2, MaxTime: 1}
	cases = append(cases, req)
	req = valid
	req.SourceAccount = "bad"
	cases = append(cases, req)
	req = valid
	req.Destination = "bad"
	cases = append(cases, req)
	req = valid
	req.Memo = stellar.Memo{Type: stellar.MemoText, Text: strings.Repeat("a", 29)}
	cases = append(cases, req)
	req = valid
	req.Asset = StellarAsset{Code: "TOOLONGCODE1"}
	cases = append(cases, req)
	req = valid
	req.Asset = StellarAsset{Code: "USD", Issuer: "bad"}
	cases = append(cases, req)
	req = valid
	req.Asset = StellarAsset{Issuer: valid.Destination}
	cases = append(cases, req)
	for _, req := range cases {
		if _, err := BuildStellarTransfer(req); err == nil {
			t.Fatalf("BuildStellarTransfer(%+v) succeeded", req)
		}
	}
}

func TestParseDecimal(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", " 1", "1 ", "-1", "+1", "1.0", "01x"} {
		if _, err := parseDecimal(value, 64); !errors.Is(err, ErrInvalidAmount) {
			t.Fatalf("parseDecimal(%q) error = %v", value, err)
		}
	}
	if got, err := parseDecimal("0001", 64); err != nil || got.String() != "1" {
		t.Fatalf("parseDecimal(0001) = %v, %v", got, err)
	}
	if _, err := parsePositiveDecimal("0", 64); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("parsePositiveDecimal(0) error = %v", err)
	}
}
