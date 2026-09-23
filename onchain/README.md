# Gacha Galaxy: borrow against graded cards on Robinhood Chain

**Gacha Galaxy is the price oracle for graded collectible cards.** This repo turns its pricing into **onchain price certificates** that lending protocols can trust, plus a vault that lends stablecoins against a tokenized graded card. It is built only on **Robinhood Chain**.

Built for the Arbitrum Open House Singapore Buildathon.

## ✅ Live on Robinhood Chain testnet (chain ID 46630), all contracts verified
| Contract | Address |
|---|---|
| AppraisalRegistry | [0x30dfBCA3978CE186e6107A93cedC7d2971d30950](https://explorer.testnet.chain.robinhood.com/address/0x30dfBCA3978CE186e6107A93cedC7d2971d30950) |
| GradedCard | [0x2Ca223844D4D118Ee92509C6c4dbD81Cb5Be1CC8](https://explorer.testnet.chain.robinhood.com/address/0x2Ca223844D4D118Ee92509C6c4dbD81Cb5Be1CC8) |
| TestUSD | [0x7e7f329325eAD4EC00807F08c5DC528cafeD77e1](https://explorer.testnet.chain.robinhood.com/address/0x7e7f329325eAD4EC00807F08c5DC528cafeD77e1) |
| CardLendingVault | [0xa9B03F4cB087d4D08d96E8e06887e362daE36400](https://explorer.testnet.chain.robinhood.com/address/0xa9B03F4cB087d4D08d96E8e06887e362daE36400) |

**6 price certificates published** for real vaulted PSA 10 slabs.
**Live borrow:** PSA 10 Rayquaza VMAX (cert 109308847) deposited, **$1,000 borrowed** against a $1,293 limit set by its certificate. [Borrow transaction](https://explorer.testnet.chain.robinhood.com/tx/0xd8910a2a6e58f3a9ecd2c15c7535fc2d63e79d09a951058524f4f4471c019491)

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
- **Bridge**: `../bridge/vela_to_registry.py` takes Vela's public attested `appraisal` event and publishes it to the registry. **Tested live:** [first bridge publish tx](https://explorer.testnet.chain.robinhood.com/tx/0x5bcddb7f009a8459531a2cfec2916390bf3f89a87a886eceaab158e34a140c94)
- The two layers are loosely coupled: the registry accepts certificates from any address holding `ATTESTER_ROLE` (today the Gacha Galaxy deployer; production: the Vela enclave's attestation key / a multisig).

### Verified end to end (2026-09-23)
| Check | Result |
|---|---|
| Vela engine unit tests (`go test ./app/...`) | ✅ 7/7 pass, incl. `TestCompsNeverLeakInPublicEvents` |
| Enclave WASM build (`make build`, TinyGo 0.39) | ✅ `build/gacha_appraisal.wasm` (1.2 MB) |
| WASM end-to-end in Wasmtime (`go test .`) | ✅ deploy → submit_comps → appraise → pledge → assess |
| Vela engine vs pricing model, all 6 slabs | ✅ 6/6 identical (band, confidence, tier, LTV) |
| **Real Vela output → bridge → Robinhood Chain** | ✅ [publish tx](https://explorer.testnet.chain.robinhood.com/tx/0xdf53ee76f0d8288d56aed73ed9e379f6ca9a7e4fb33d8f441fd704bad9c854ca); vault reads it, $1,293 limit |

Reproduce: `(cd .. && go run ./cmd/vela2rh -cert 109308847 2>/dev/null | grep '^{' > onchain/a.json) && python3 ../bridge/vela_to_registry.py a.json --grader PSA --registry <REGISTRY> --send`

## The problem
Graded cards are a fast-growing real-world asset, and platforms such as Collector Crypt, Courtyard and Beezie already tokenize vaulted slabs. **You can't borrow against them**, because lenders have no trusted, neutral price. Appraisals today are manual, slow and based on trust.

## How it works
1. **AppraisalRegistry** stores a *price certificate* for each slab, keyed by grader + cert number: a fair-value band, a confidence tier, a risk tier and a max loan-to-value. Only the authorised Gacha Galaxy attester can publish.
2. **GradedCard** is a testnet ERC-721 standing in for a tokenized vaulted slab.
3. **CardLendingVault**: deposit a card, and the vault **reads its certificate** and lets you borrow up to `fmvLow × LTV`.
   - A certificate older than 7 days blocks new borrowing.
   - Ineligible or rejected cards get LTV 0.
   - If a fresh appraisal drops the value past the liquidation threshold (LTV + 10 points), anyone can repay the debt and take the card.
4. **TestUSD**: a 6-decimal test stablecoin used as the loan asset.

## Pricing data
Each certificate blends two live sources (`data/blend.py`):
- **Gacha Galaxy oracle**: fair values for same-grade listings across Courtyard, Collector Crypt and Beezie (`data/build_seed_gg.py`)
- **Collector Crypt public API**: insured values and live asks for other vaulted slabs of the same card and grade (`data/build_seed.py`)

The Gacha Galaxy appraisal model: take the median, drop anything more than 60% away from it (sanity filter), then set the band from the spread, score confidence from the comp count and dispersion, and map that to a tier and LTV (A = 50%, B = 35%, C = 20%). Fewer than 3 comps means **ineligible**.

| Card (PSA 10) | Cert | Comps | Fair value | Max loan |
|---|---|---|---|---|
| Rayquaza VMAX 218, Evolving Skies | 109308847 | 20 | $2,587 – $3,213 | $1,293 |
| Pikachu & Zekrom GX SM168 | 118973623 | 15 | $2,875 – $3,525 | $1,438 |
| Lugia V 186, Silver Tempest | 100863346 | 14 | $1,216 – $1,446 | $608 |
| Giratina VSTAR GG69, Crown Zenith | 154790134 | 47 | $524 – $776 | $262 |
| Rayquaza VMAX TG20, Silver Tempest | 153708045 | 29 | $340 – $480 | $170 |
| Umbreon VMAX TG23, Brilliant Stars | 111992917 | 27 | $249 – $301 | $125 |

> **Price basis:** oracle fair values, insured values and live asks. These are **not completed sales**. Next step: add graded sold comps (eBay PSA sales).

## Repo layout (this repo)
```
onchain/src test script   Robinhood Chain contracts (Foundry)   <- you are here
onchain/data/             pricing scripts (Gacha Galaxy oracle + Collector Crypt)
bridge/                   Vela appraisal -> AppraisalRegistry publisher
(repo root)               confidential appraisal engine (Horizen Vela WASM app)
```

## Run it
```bash
curl -L https://foundry.paradigm.xyz | bash && foundryup
forge install foundry-rs/forge-std OpenZeppelin/openzeppelin-contracts@v5.1.0 --no-git
forge test                                   # 13 tests incl. fuzz
cp .env.example .env && source .env          # testnet-only PRIVATE_KEY
forge script script/Deploy.s.sol --rpc-url robinhood_testnet --broadcast
forge script script/Seed.s.sol   --rpc-url robinhood_testnet --broadcast
```
Refresh the data: `python3 data/build_seed.py && python3 data/build_seed_gg.py && python3 data/blend.py && python3 data/gen_seed_script.py`

## Try it (cast)
```bash
R=0x30dfBCA3978CE186e6107A93cedC7d2971d30950; V=0xa9B03F4cB087d4D08d96E8e06887e362daE36400
cast call $R "getAppraisalByCert(string,string)((uint64,uint64,uint64,uint16,uint16,uint16,uint8,uint8,bool,uint40,address))" PSA 109308847 --rpc-url https://rpc.testnet.chain.robinhood.com
cast call $V "borrowLimit(uint256)(uint256)" 1 --rpc-url https://rpc.testnet.chain.robinhood.com
```

## Demo scope (honest limits)
No interest accrual, a single loan asset, and a single attester key (production: multisig with independent attesters). The card token is a testnet stand-in for a vaulted-slab token.

## Roadmap (Founder House, Oct 23–25)
1. Certificates served from the live Gacha Galaxy app (24K+ cards)
2. $GG staking tiers for certificate access
3. Robinhood Chain mainnet
4. A pilot with a lender or tokenization platform
