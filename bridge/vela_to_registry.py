#!/usr/bin/env python3
"""
Bridge: Vela confidential appraisal  ->  Robinhood Chain AppraisalRegistry

The Vela app (privacy layer) emits a PUBLIC "appraisal" app event carrying only the
attested AppraisalOut JSON. Raw comps never leave the enclave. This script takes that
JSON and publishes it as a price certificate on Robinhood Chain, which the
CardLendingVault reads.

  python3 vela_to_registry.py appraisal.json --grader PSA --rpc $RPC --registry $REG            # dry run: prints calldata
  python3 vela_to_registry.py appraisal.json --grader PSA --rpc $RPC --registry $REG --send     # broadcast (needs PRIVATE_KEY env)

AppraisalOut fields (see types.go): certId, fmvLow, fmvPoint, fmvHigh (USD cents, Uint256 as
decimal or 0x-hex string), confidenceTier (REAL|MODEST|NARRATIVE), confidenceScore (0-100),
eligible, ltvBps, riskTier (A|B|C|REJECT), compCount.
Requires Foundry `cast`. The sending key must hold ATTESTER_ROLE on the registry.
"""
import argparse, json, os, subprocess, sys

CONF = {"NARRATIVE": 1, "MODEST": 2, "REAL": 3}
RISK = {"REJECT": 1, "C": 2, "B": 3, "A": 4}
SIG = "publish(string,string,uint64,uint64,uint64,uint16,uint16,uint16,uint8,uint8,bool)"

def num(v):
    if isinstance(v, int): return v
    s = str(v).strip()
    return int(s, 16) if s.lower().startswith("0x") else int(s)

def to_args(a, grader):
    low, pt, high = num(a["fmvLow"]), num(a["fmvPoint"]), num(a["fmvHigh"])
    if not (0 < low <= pt <= high): raise ValueError(f"invalid FMV band {low}/{pt}/{high}")
    eligible = bool(a.get("eligible")) and a.get("riskTier") != "REJECT"
    ltv = int(a.get("ltvBps", 0)) if eligible else 0
    return [grader, str(a["certId"]), str(low), str(pt), str(high), str(ltv),
            str(min(int(a.get("confidenceScore", 0)) * 10, 1000)),  # 0-100 -> 0-1000
            str(int(a.get("compCount", 0))), str(CONF[a["confidenceTier"]]), str(RISK[a["riskTier"]]),
            "true" if eligible else "false"]

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("appraisal", help="AppraisalOut JSON file (or '-' for stdin); may be the app event {type,appraisal}")
    ap.add_argument("--grader", required=True, help="PSA | CGC | BGS | SGC | TAG (Vela keeps it in CardProfile)")
    ap.add_argument("--registry", required=True)
    ap.add_argument("--rpc", default=os.environ.get("RH_TESTNET_RPC", "https://rpc.testnet.chain.robinhood.com"))
    ap.add_argument("--send", action="store_true")
    o = ap.parse_args()
    raw = json.load(sys.stdin if o.appraisal == "-" else open(o.appraisal))
    a = raw.get("appraisal", raw)
    args = to_args(a, o.grader.upper())
    if not o.send:
        cd = subprocess.check_output(["cast", "calldata", SIG, *args], text=True).strip()
        print(json.dumps({"to": o.registry, "function": SIG, "args": args, "calldata": cd}, indent=2)); return
    pk = os.environ.get("PRIVATE_KEY") or sys.exit("PRIVATE_KEY not set")
    out = subprocess.check_output(["cast", "send", o.registry, SIG, *args, "--private-key", pk, "--rpc-url", o.rpc, "--json"], text=True)
    print(json.dumps({"published": a["certId"], "tx": json.loads(out)["transactionHash"]}, indent=2))

if __name__ == "__main__":
    main()
