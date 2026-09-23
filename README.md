# Gacha Galaxy — Confidential Appraisal Engine (Vela WASM App)

Private collateral valuation for high-value graded collectible cards, built as a
Horizen Vela WASM guest application.

**What it does:** a lender or collector gets an attested appraisal (FMV band,
confidence tier, collateral eligibility, LTV / risk tier) for a slabbed card —
without the raw dealer comps, portfolio contents, or Gacha Galaxy's proprietary
scoring logic ever leaving the enclave.

```
 Private inputs (never leave the enclave)        Attested output (public)
 ────────────────────────────────────────        ─────────────────────────────
 • dealer / marketplace comparable sales   ──►   • FMV low / point / high
 • collector portfolio composition         ──►   • confidence tier (REAL/MODEST/NARRATIVE)
 • scoring weights, LTV ladder, gates      ──►   • collateral eligibility
                                                 • LTV (bps) + risk tier (A/B/C/REJECT)
```

## Architecture: two layers, one product
```
  PRIVACY LAYER (Horizen Vela TEE)                    LENDING LAYER (Robinhood Chain)
  ──────────────────────────────────                  ─────────────────────────────────────
  private dealer / marketplace comps  ─┐
  Gacha Galaxy scoring model          ─┼─► attested   AppraisalRegistry  (price certificate)
  (never leave the enclave)           ─┘   appraisal          │  read by
                                              │               ▼
                                     bridge/vela_to_registry.py   CardLendingVault ──► borrow tUSD
                                     (publishes as attester)       against a GradedCard token
```
- **Privacy layer**: computes the fair-value band, confidence, eligibility and LTV. Raw comps stay private.
- **Lending layer**: stores the certificate onchain and lends against it.
- **Bridge**: `bridge/vela_to_registry.py` takes Vela's public attested `appraisal` event and publishes it to the registry. **Tested live:** [first bridge publish tx](https://explorer.testnet.chain.robinhood.com/tx/0x5bcddb7f009a8459531a2cfec2916390bf3f89a87a886eceaab158e34a140c94)
- The two layers are loosely coupled: the registry accepts certificates from any address holding `ATTESTER_ROLE` (today the Gacha Galaxy deployer; production: the Vela enclave's attestation key / a multisig).

### Verified end to end (2026-09-23)
| Check | Result |
|---|---|
| Vela engine unit tests (`go test ./app/...`) | ✅ 7/7 pass, incl. `TestCompsNeverLeakInPublicEvents` |
| Enclave WASM build (`make build`, TinyGo 0.39) | ✅ `build/gacha_appraisal.wasm` (1.2 MB) |
| WASM end-to-end in Wasmtime (`go test .`) | ✅ deploy → submit_comps → appraise → pledge → assess |
| Vela engine vs pricing model, all 6 slabs | ✅ 6/6 identical (band, confidence, tier, LTV) |
| **Real Vela output → bridge → Robinhood Chain** | ✅ [publish tx](https://explorer.testnet.chain.robinhood.com/tx/0xdf53ee76f0d8288d56aed73ed9e379f6ca9a7e4fb33d8f441fd704bad9c854ca); vault reads it, $1,293 limit |

Reproduce: `go run ./cmd/vela2rh -cert 109308847 2>/dev/null | grep '^{' > a.json && python3 bridge/vela_to_registry.py a.json --grader PSA --registry <REGISTRY> --send`

## Lending layer on Robinhood Chain (live)
The attested appraisals feed a lending layer on **Robinhood Chain testnet**: see `onchain/` (contracts, tests, pricing data) and `bridge/` (publisher). All contracts are verified, 6 certificates are published, and a live $1,000 borrow has been made against a PSA 10 card. Addresses are in `onchain/README.md`.

## Privacy boundary

| Stays private (inside TEE) | Settles / attested (onchain) |
|---|---|
| Raw comp prices + sources | FMV band (low/point/high) |
| Collector portfolio contents | Confidence tier + score |
| Scoring weights, sanity gate, LTV ladder | Collateral eligibility |
| Per-card comp count is public; comps are not | LTV bps + risk tier |

Even the **deanonymization report** (authority flow) exposes only the latest
attested appraisal per card — never raw comps or portfolios.

## Repo layout

```
main.go                  # WASM exports (thin bridge)
app/
  types.go               # state, instructions, events, report types
  app.go                 # guest entry points: deploy / process_request / deposit
  appraise.go            # valuation core (enclave-private IP)
  app_test.go            # unit tests (native go)
internal/
  subtype/               # bytes32 event-subtype helper (vendored)
  reqtype/               # request-type constants (vendored)
integration_test.go      # end-to-end: loads build/*.wasm into Wasmtime and
                         # drives deploy→submit_comps→appraise→pledge→assess
Makefile                 # tinygo build (dev + production)
cmd/vela2rh/             # runs the engine on real comps, prints the public attested appraisal
bridge/                  # publishes that appraisal to Robinhood Chain AppraisalRegistry
onchain/                 # Robinhood Chain lending layer (Foundry)
```

## Instructions (encrypted payloads to `process_request`)

| `type` | Payload | Auth | Effect |
|---|---|---|---|
| `submit_comps` | `{submitComps:{certId, grader, grade, comps:[{source,price,timestamp}]}}` | authorized data source | append private comps (ring-bounded) |
| `appraise` | `{appraise:{certId}}` | anyone | run valuation → attested `AppraisalOut` |
| `pledge` | `{pledge:{certIds:[...]}}` | portfolio owner | pledge appraised cards as collateral |
| `assess` | `{assess:{}}` | portfolio owner | aggregate → max loan, eligible cards, risk |
| `add_source` | `{addSource:{source}}` | admin | authorize a new comp submitter |

Prices are USD **cents** encoded as `Uint256` (hex `"0x…"` in JSON).

## Build & test

Prereqs: Go ≥1.24, TinyGo (`tinygo`), and for the integration test a working
`wasmtime` (via `wasmtime-go`).

```bash
# unit tests (valuation logic, native)
go test ./app/...

# build the WASM module (development)
make build                 # → build/gacha_appraisal.wasm

# build production (optimized, no debug symbols)
make production_build      # → production_build/gacha_appraisal.wasm

# end-to-end: instantiate the compiled module in Wasmtime and run the full flow
go test -run TestWasmEndToEnd -v .
```

## Deploy to the local Vela environment

The local stack (Anvil + manager + authority service) runs from the
[`vela-starterkit`](https://github.com/HorizenOfficial/vela-starterkit)
`dockerfiles/` compose setup. Deploy this module's `build/gacha_appraisal.wasm`
as the app; constructor params = `{"dataSources":[<hex>...], "maxComps":50}`.

## Notes for the Vela team (Deliverable 1)

- The guest deliberately depends only on `vela-common-go` (not the host `vela`
  module). Importing `vela` pulls go-ethereum into the TinyGo compile and OOMs
  under ~2GB; vendoring the two tiny helpers (`subtype`, request-type
  constants) keeps the WASM build lightweight. Worth documenting in the DevDocs.
- `time.Now()` is used for appraisal timestamps; if deterministic replay across
  enclave restarts matters, consider a host-supplied timestamp.
