"""
Build the Buildathon seed set from Collector Crypt's public marketplace API (no key needed).

For each target card we pull every vaulted slab of the same item + grade.
  subject  = one real slab (grader + cert number printed on the slab)
  comps    = every OTHER slab of that item/grade: its insured value, plus its
             live ask if listed

IMPORTANT, and stated in the README too:
  these are Collector Crypt insured valuations and live asking prices,
  NOT completed sales. Swap in sold comps (PokeTrace / TCG Price Lookup / PSA APR)
  once an API key is available. The appraisal maths below does not change.

The valuation is a line-for-line port of app/appraise.go (defaultScoring):
median -> 60% deviation sanity filter -> median of kept -> band = +/- spread/2
-> confidence score -> tier -> LTV.
"""
import json, time, statistics, urllib.request, urllib.parse, datetime

API = "https://api.collectorcrypt.com/marketplace"
UA = {"User-Agent": "gacha-galaxy-buildathon/1.0"}

TARGETS = [
    # (search text, exact itemName to match)
    ("Rayquaza Vmax Evolving Skies", "2021 #218 Full Art/Rayquaza Vmax PSA 10 Sword & Shield Evolving Skies"),
    ("Pikachu & Zekrom", "2019 #SM168 Full Art/Pikachu & Zekrom GX PSA 10 SM Black Star Promo"),
    ("Lugia V Silver Tempest", "2022 #186 Full Art/Lugia V PSA 10 Sword & Shield Silver Tempest"),
    ("Giratina Vstar Crown Zenith", "2023 #GG69 Full Art/Giratina Vstar PSA 10 Sword and Shield Crown Zenith"),
    ("Rayquaza Vmax Silver Tempest", "2022 #TG20 Full Art/Rayquaza Vmax PSA 10 Sword & Shield Silver Tempest"),
    ("Umbreon Vmax Brilliant Stars", "2022 #TG23 Full Art/Umbreon Vmax PSA 10 Sword & Shield Brilliant Stars"),
]

# ---- port of appraise.go defaultScoring ----
CFG = dict(MaxDevBps=6000, ConfReal=70, ConfModest=40, MinEligible=40, MinComps=3,
           LtvA=5000, LtvB=3500, LtvC=2000)

def median_int(v):
    s = sorted(v); n = len(s); m = n // 2
    return s[m] if n % 2 else (s[m-1] + s[m]) // 2

def rel_dev_bps(p, ref):
    return 0 if ref == 0 else abs(p - ref) * 10000 // ref

def spread_bps(v, med):
    return 0 if not v or med == 0 else (max(v) - min(v)) * 10000 // med

def conf_score(n, spread):
    base = 60 if n >= 10 else 45 if n >= 5 else 30 if n >= 3 else 15 if n >= 1 else 0
    pen = min(spread * 40 // 6000, 40)
    s = base + 40 - pen
    if n < 2 and s > CFG["ConfModest"] - 1: s = CFG["ConfModest"] - 1
    elif n < 3 and s > CFG["ConfReal"] - 1: s = CFG["ConfReal"] - 1
    return max(0, min(100, s))

def appraise(prices_cents):
    out = dict(compCount=len(prices_cents))
    if not prices_cents:
        return dict(out, eligible=False, confidenceTier="NARRATIVE", riskTier="REJECT", ltvBps=0)
    med = median_int(prices_cents)
    kept = [p for p in prices_cents if rel_dev_bps(p, med) <= CFG["MaxDevBps"]]
    out["sanityRejected"] = len(kept) != len(prices_cents)
    out["rejectedCount"] = len(prices_cents) - len(kept)
    if not kept: kept = prices_cents
    point = median_int(kept)
    spread = spread_bps(kept, point)
    half = point * (spread // 2) // 10000
    low, high = max(point - half, 0), point + half
    score = conf_score(len(kept), spread)
    tier = "REAL" if score >= CFG["ConfReal"] else "MODEST" if score >= CFG["ConfModest"] else "NARRATIVE"
    eligible = score >= CFG["MinEligible"] and len(kept) >= CFG["MinComps"]
    if not eligible: risk, ltv = "REJECT", 0
    elif score >= CFG["ConfReal"]: risk, ltv = "A", CFG["LtvA"]
    elif score >= CFG["ConfModest"]: risk, ltv = "B", CFG["LtvB"]
    else: risk, ltv = "C", CFG["LtvC"]
    return dict(out, fmvLow=low, fmvPoint=point, fmvHigh=high, spreadBps=spread, keptCount=len(kept),
                confidenceScore=score, confidenceTier=tier, eligible=eligible, riskTier=risk, ltvBps=ltv)

def fetch(search):
    q = urllib.parse.urlencode({"search": search, "step": 1000})
    req = urllib.request.Request(f"{API}?{q}", headers=UA)
    with urllib.request.urlopen(req, timeout=60) as r:
        return json.load(r).get("filterNFtCard", [])

def grader_code(c):
    g = (c.get("gradingCompany") or "").upper()
    return "BGS" if g == "BECKETT" else g

pulled_at = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
cards = []
for search, item in TARGETS:
    rows = [c for c in fetch(search) if c.get("itemName", "").strip() == item]
    time.sleep(1)
    rows = [c for c in rows if c.get("gradingID")]
    if len(rows) < 4:
        print("SKIP (too few slabs):", item, len(rows)); continue
    rows.sort(key=lambda c: float(c.get("insuredValue") or 0))
    subject = rows[len(rows) // 2]             # a real, mid-valued slab
    comps = []
    for c in rows:
        if c is subject: continue
        if c.get("insuredValue"):
            comps.append(dict(kind="insured_value", cents=int(round(float(c["insuredValue"]) * 100)),
                              cert=c["gradingID"], nft=c["nftAddress"]))
        if c.get("listing") and c["listing"].get("price"):
            comps.append(dict(kind="live_ask", cents=int(round(float(c["listing"]["price"]) * 100)),
                              cert=c["gradingID"], nft=c["nftAddress"]))
    a = appraise([x["cents"] for x in comps])
    cards.append(dict(
        name=item, grader=grader_code(subject), certId=subject["gradingID"], grade=str(subject.get("gradeNum") or subject.get("grade")),
        subjectNft=subject["nftAddress"], subjectInsuredUsd=float(subject.get("insuredValue") or 0),
        subjectAskUsd=(subject.get("listing") or {}).get("price"),
        solscan=f"https://solscan.io/address/{subject['nftAddress']}",
        slabsFound=len(rows), comps=comps, appraisal=a))
    print(f"{item[:60]:60} slabs={len(rows):3} comps={len(comps):3} "
          f"FMV ${a.get('fmvLow',0)/100:,.0f}-${a.get('fmvHigh',0)/100:,.0f} (pt ${a.get('fmvPoint',0)/100:,.0f}) "
          f"{a['confidenceTier']} score={a.get('confidenceScore')} LTV={a['ltvBps']/100:.0f}% risk={a['riskTier']} rejected={a.get('rejectedCount',0)}")

json.dump(dict(source="Collector Crypt public marketplace API (api.collectorcrypt.com/marketplace)",
               priceBasis="insured valuations + live asking prices of other vaulted slabs of the same item and grade; NOT completed sales",
               pulledAt=pulled_at, scoring=CFG, cards=cards), open("seed_cards.json", "w"), indent=2)
print("wrote seed_cards.json with", len(cards), "cards")
