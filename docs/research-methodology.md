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

## Trading signals

Model `trend-structure-1.0`, rules `structure-1.0` (backend `internal/trading`). Signals are deterministic rule outputs over public price data. They are not predictions and not investment advice.

### Data

- Source: DeFiLlama coins API (`coins.llama.fi/chart`), an aggregate price across venues. Venue is reported as "DeFiLlama aggregate", quote currency USD.
- Timeframes: `4h` candles built from hourly samples (480 samples ≈ 120 candles) and `1d` candles from 4-hour samples (480 samples ≈ 80 candles). No sub-hour data exists, so no `1h` timeframe is offered.
- Candles are approximated: open/close are the first/last sample in the bucket, high/low the sample extremes. Intrabar wicks between samples are not captured, so ranges and stop hits are understated. A bucket needs all but one of its samples, and the still-forming bar is excluded.
- Volume, CVD, order-book depth, open interest, funding and liquidations are **not available** from this source and are returned as null with quality `unavailable`, never estimated.
- Universe: 20 assets across the registry chains with a static liquidity tier (Tier 1 majors, Tier 2 established mid-caps, Tier 3 smaller ecosystem tokens), maintained manually.

### Score

An asset is ranked only with at least 60 bars, at least 80% bar coverage and a last sample no older than 3 sample intervals. The composite score in [-1, 1] is:

| Component | Weight | Definition |
|---|---|---|
| Momentum | 0.40 | tanh(20-bar return ÷ 20-bar realized volatility) |
| Trend | 0.35 | +1 if close > EMA20 > EMA50, −1 for the mirror, otherwise ±0.5 by the sign of EMA20 − EMA50 |
| Structure | 0.25 | +1 / −1 for the direction of the latest break of structure |

Direction is bullish when the composite is ≥ 0.15, bearish when ≤ −0.15, neutral otherwise. The published score is |composite| × 100. It ranks assets against each other and is not a probability. The regime is bullish or bearish when at least 60% of ranked assets agree, mixed otherwise.

### Structure rules

- Swings: fractal pivots with 2 bars on each side.
- Break of structure (BOS): a close beyond the latest unbroken swing in the direction of the previous break. A break against it is a change of character (CHoCH).
- Order block: the last opposite-colored candle (within 10 bars) before the impulse that produced a break.
- Fair value gap: candle[i−2].high < candle[i].low (bullish) or candle[i−2].low > candle[i].high (bearish).
- Zone status on later candles: invalidated when a close goes through the far edge, mitigated when price trades through the midpoint, tested when price touches the zone, expired after 60 untouched bars, fresh otherwise.
- Liquidity levels are unswept swing highs (buy-side) and swing lows (sell-side).

### Scenarios, journal and backtests

- Scenario: when a fresh or tested order block in the signal direction lies within 3 ATR, it is the suggested entry area and the invalidation sits 0.25 ATR beyond its far edge. Otherwise the entry is the last close with a 1.5 ATR stop. Targets are at 1.5R and 3R. A zone entry is a limit-order suggestion that may never fill.
- Journal: the top five signals per direction are recorded once per bar (immutable, idempotent id). Each is graded at the price that actually traded: entry at the signal bar's close, stop 1.5 ATR, target 3R, at most 12 bars (4h) or 10 bars (1d). Inside a bar the stop is checked before the target. Outcomes are computed on read from price data and never edited.
- Backtests: a walk-forward replay of the same rules. At each bar only prior data is used, a trade opens at |score| ≥ 40 when the asset has no open trade, and results are net of 10 bps round-trip cost. Trades whose horizon runs past the data are excluded. The benchmark is equal-weight buy-and-hold over the same window. Past results do not predict future results.
