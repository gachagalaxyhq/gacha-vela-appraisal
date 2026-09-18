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
