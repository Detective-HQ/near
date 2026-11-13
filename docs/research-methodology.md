# Research data and scenario methodology

## Fixture and live-source policy

In the default `MARKET_DATA_MODE=fixture`, market and ecosystem values are illustrative test data. They are not queried from a provider, indexer, chain RPC, or exchange. The API returns `source: "Demo fixture"`, `classification: "illustrative"`, `observedAt: null`, and a fixture capture date.

With `MARKET_DATA_MODE=live`, the optional DeFiLlama adapter provides NEAR TVL, DEX volume, stablecoin supply, protocol fees, protocol revenue, and a TVL history. Values include a source URL, retrieval time, unit, scope, status, and methodology version. The interface marks stale and unavailable values; it does not silently substitute fixtures. Provider coverage and attribution can change, so the metric definition and source response must be reviewed before including values in a formal thesis.

This connector does not source daily active accounts or transactions, and it does not persist raw-input snapshots. Do not cite fixture values in an investment thesis. A production research pipeline still needs immutable input snapshots, checksum/version tracking, scheduled ingestion, source-specific validation, and reviewed methodology.

## Metric dictionary

| Metric | Unit | Scope | Interpretation guardrail |
| --- | --- | --- | --- |
| Daily active accounts | accounts/day | NEAR Protocol | Define activity and account filtering; activity may include bots or incentives. |
| Daily transactions | transactions/day | NEAR Protocol | Define included transaction/receipt types and deduplication. |
| TVL | USD | NEAR DeFi | Record protocol coverage, price source, and double-counting method. |
| DEX volume | USD/day | NEAR DeFi | Gross volume is not fees or revenue. |
| Stablecoin supply | USD | NEAR ecosystem | Supply does not equal trading liquidity. |
| Protocol fees | USD/day | NEAR DeFi | Separate user-paid fees from revenue retained by a protocol. |
| Protocol revenue | USD/day | NEAR DeFi | Report separately from gross fees; provider coverage and attribution vary. |

## Scenario calculation

The UI computes:

```text
annual captured fees
  = daily volume
  × (1 + annual activity growth)
  × fee rate
  × value-capture share
  × 365
```

Each case exposes daily volume, fee rate, value-capture share, and activity-growth assumptions. Two controls apply a uniform percentage-point adjustment to activity growth and capture share. The output is a simple scenario calculation, not an audited forecast. It does not calculate token price, market capitalization, or investment return.

Production research models should version assumptions, save input snapshots, define missing-data handling, include source dates, and test sensitivity to each assumption independently.
