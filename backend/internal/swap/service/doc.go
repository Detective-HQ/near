// Package service implements the core swap orchestration layer.
//
// # 6.1 Quote vs Swap Lifecycle Architecture
//
// In Universal DeFi Intelligence (UDI), quoting and swap execution have distinct
// lifecycles, states, and persistence boundaries:
//
//  1. Dry Quoting (POST /api/v1/quotes):
//     - Operates strictly on the quotes table.
//     - Never creates a row in the swaps table or emits swap events.
//     - In demo mode, generates deterministic simulated quotes (mode: simulation).
//     - In live mode, requests non-executable quotes from the upstream provider
//     (mode: provider_dry_quote) via GetDryQuote.
//     - Normalized quotes are persisted for auditing, inspection, and subsequent
//     swap creation references.
//
//  2. Executable Quoting & Swap Creation (POST /api/v1/swaps):
//     - Requires a valid base quote ID and a client-supplied Idempotency-Key
//     (between 8 and 128 characters).
//     - The request payload is canonicalized and hashed (SHA-256) to guarantee
//     strict idempotency: replaying an identical key and payload returns the
//     existing swap with HTTP 200 (replayed=true); replaying the same key with
//     a different payload returns HTTP 409 IDEMPOTENCY_CONFLICT.
//     - In live mode, the service checks the live execution safety gate
//     (ENABLE_LIVE_EXECUTION=true) and checks chain registry route capabilities
//     (SwapEnabled, DepositSupported) before requesting a fresh executable quote
//     from the provider (GetExecutableQuote).
//     - Atomically inserts the persistent swap row into PostgreSQL alongside its
//     initial audit event in one database transaction.
//     - Live swaps begin in state AWAITING_USER_DEPOSIT with the provider's
//     deposit address and optional deposit memo.
//
//  3. Deposit Capture (POST /api/v1/swaps/{id}/deposit):
//     - Records a user-submitted source-chain transaction hash.
//     - Validates format per origin chain family (EVM 0x hex, Solana base58, etc.).
//     - Rejects duplicate hashes attached to different swaps, and rejects unfunded
//     expired swaps.
//     - If the same transaction hash is reported repeatedly for the same swap,
//     returns duplicate=true with HTTP 200 without creating a duplicate record.
//     - Atomically records the transaction in swap_transactions, populates
//     swaps.source_tx_hash, and transitions AWAITING_USER_DEPOSIT -> DEPOSIT_SUBMITTED.
//     - Asynchronously submits the transaction notification to the upstream
//     provider via SubmitDepositTx. If the upstream provider request fails or
//     times out, the local transaction and state change remain intact; background
//     polling against the deposit address remains the authority for final settlement.
//
//  4. Cancellation & Expiry (POST /api/v1/swaps/{id}/cancel & Poller Expiry):
//     - A user may cancel an unfunded quote while in state AWAITING_USER_DEPOSIT
//     as long as no source transaction hash has been recorded.
//     - If the provider deadline has passed, the swap transitions to EXPIRED.
//     - If active and unfunded, the swap transitions to CANCELLED.
//     - Funded swaps (where a source transaction has been submitted or detected)
//     cannot be cancelled and are never silently expired.
//
//  5. Custody Boundary (Non-Custodial):
//     - The backend never holds private keys, signs transactions, or executes
//     transfers on behalf of users.
//     - The backend provides validated deposit addresses and unsigned payloads;
//     the user's wallet signs and broadcasts the transaction.
package service
