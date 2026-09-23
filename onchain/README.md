# Gacha Galaxy: borrow against graded cards on Robinhood Chain

Open House Singapore Buildathon build. Target: **Robinhood Chain testnet (chain ID 46630)**.

## What it does
1. **AppraisalRegistry** stores a *price certificate* for a real graded slab (grader + cert number): fair-value band, confidence, risk tier and max loan-to-value. Only an authorised attester (Gacha Galaxy) can publish.
2. **GradedCard** is a testnet ERC-721 standing in for a tokenized, vaulted slab.
3. **CardLendingVault**: deposit a card, and the vault **reads its certificate** and lets you borrow stablecoins up to `fmvLow × LTV`. If a fresh appraisal drops the value past the liquidation threshold, anyone can repay the debt and take the card. Stale certificates (older than 7 days) block new borrowing.
4. **TestUSD**: a 6-decimal test stablecoin used as the loan asset.

## Data
`data/build_seed.py` pulls **real vaulted slabs** from Collector Crypt's public marketplace API (no key needed) and appraises them. The appraisal code is a direct port of `appraise.go` (median, 60% sanity filter, confidence score, tier, LTV).

> ⚠️ **Price basis:** the comps are Collector Crypt **insured valuations + live asking prices** of other slabs of the same card and grade. They are **not completed sales**. Swap in sold comps (PokeTrace / TCG Price Lookup / PSA auction prices) when an API key is available; the appraisal maths stays the same.

| Card (PSA 10) | Cert | Fair value | Max loan (50% LTV) |
|---|---|---|---|
| Rayquaza VMAX 218, Evolving Skies | 109308847 | $2,725 – $3,075 | $1,363 |
| Pikachu & Zekrom GX SM168 | 118973623 | $2,875 – $3,525 | $1,438 |
| Lugia V 186, Silver Tempest | 100863346 | $1,269 – $1,393 | $634 |
| Giratina VSTAR GG69, Crown Zenith | 154790134 | $550 – $610 | $275 |
| Rayquaza VMAX TG20, Silver Tempest | 153708045 | $375 – $415 | $188 |
| Umbreon VMAX TG23, Brilliant Stars | 111992917 | $263 – $282 | $131 |

## Run it
```bash
curl -L https://foundry.paradigm.xyz | bash && foundryup
cd onchain && forge test                       # 13 tests incl. fuzz
cp .env.example .env                           # add PRIVATE_KEY (testnet-only wallet!)
source .env
# 1. testnet ETH: https://faucet.testnet.chain.robinhood.com
forge script script/Deploy.s.sol --rpc-url robinhood_testnet --broadcast
forge script script/Seed.s.sol   --rpc-url robinhood_testnet --broadcast
# addresses -> deployments/46630.json ; explorer: https://explorer.testnet.chain.robinhood.com
```

Refresh the data: `python3 data/build_seed.py && python3 data/gen_seed_script.py`.

## Try the borrow flow (cast)
```bash
R=$(jq -r .registry deployments/46630.json); V=$(jq -r .vault deployments/46630.json); C=$(jq -r .cards deployments/46630.json)
cast call $R "getAppraisalByCert(string,string)((uint64,uint64,uint64,uint16,uint16,uint16,uint8,uint8,bool,uint40,address))" PSA 109308847 --rpc-url robinhood_testnet
cast send $C "approve(address,uint256)" $V 1 --private-key $PRIVATE_KEY --rpc-url robinhood_testnet
cast send $V "deposit(uint256)" 1        --private-key $PRIVATE_KEY --rpc-url robinhood_testnet
cast call $V "borrowLimit(uint256)(uint256)" 1 --rpc-url robinhood_testnet     # 1362.565 tUSD
cast send $V "borrow(uint256,uint256)" 1 1000000000 --private-key $PRIVATE_KEY --rpc-url robinhood_testnet  # borrow $1,000
```

## Demo scope (honest limits)
No interest accrual, a single loan asset, and one attester key (production: Vela TEE attestation / multisig). The card token is a testnet stand-in for a vaulted-slab token.
