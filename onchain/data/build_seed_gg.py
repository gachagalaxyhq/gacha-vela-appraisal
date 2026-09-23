"""
PRIMARY data source: Gacha Galaxy's own live oracle (gachagalaxy.io/app, public, no key).

  subject = a real vaulted slab (grader + cert number) from Collector Crypt's public API
  comps   = Gacha Galaxy oracle fair values for every live listing of the SAME card at the
            SAME grade, across the marketplaces Gacha Galaxy tracks (Courtyard, Collector Crypt, Beezie...)
Appraisal = the same port of appraise.go used everywhere else (median, 60% sanity filter,
confidence, tier, LTV). Cards with fewer than 3 same-grade comps come out INELIGIBLE (LTV 0):
the engine refuses to lend on thin data.

Cross-check: seed_cards_collectorcrypt.json (Collector Crypt insured values + asks).
Price basis: oracle fair values of live listings. NOT completed sales.
"""
import re, html, json, time, urllib.request, datetime

UA = {"User-Agent": "Mozilla/5.0 gacha-buildathon"}
BASE = "https://www.gachagalaxy.io"
MAP = {  # cert -> Gacha Galaxy oracle card ids (same card, listed on different platforms)
    "109308847": [8922],                    # Rayquaza VMAX #218 Evolving Skies
    "100863346": [6906],                    # Lugia V #186 Silver Tempest
    "154790134": [798, 2638, 6472, 16085],  # Giratina VSTAR #GG69 Crown Zenith
    "153708045": [3492],                    # Rayquaza VMAX #TG20 Silver Tempest
    "111992917": [11822, 2419],             # Umbreon VMAX #TG23 Brilliant Stars
}
CFG = dict(MaxDevBps=6000, ConfReal=70, ConfModest=40, MinEligible=40, MinComps=3, LtvA=5000, LtvB=3500, LtvC=2000)

def median_int(v):
    s = sorted(v); n = len(s); m = n // 2
    return s[m] if n % 2 else (s[m-1] + s[m]) // 2
def rel_dev_bps(p, ref): return 0 if ref == 0 else abs(p - ref) * 10000 // ref
def spread_bps(v, med): return 0 if not v or med == 0 else (max(v) - min(v)) * 10000 // med
def conf_score(n, spread):
    base = 60 if n >= 10 else 45 if n >= 5 else 30 if n >= 3 else 15 if n >= 1 else 0
    s = base + 40 - min(spread * 40 // 6000, 40)
    if n < 2 and s > CFG["ConfModest"] - 1: s = CFG["ConfModest"] - 1
    elif n < 3 and s > CFG["ConfReal"] - 1: s = CFG["ConfReal"] - 1
    return max(0, min(100, s))
def appraise(p):
    med = median_int(p)
    kept = [x for x in p if rel_dev_bps(x, med) <= CFG["MaxDevBps"]] or p
    point = median_int(kept); spread = spread_bps(kept, point)
    half = point * (spread // 2) // 10000
    score = conf_score(len(kept), spread)
    tier = "REAL" if score >= CFG["ConfReal"] else "MODEST" if score >= CFG["ConfModest"] else "NARRATIVE"
    eligible = score >= CFG["MinEligible"] and len(kept) >= CFG["MinComps"]
    if not eligible: risk, ltv = "REJECT", 0
    elif score >= CFG["ConfReal"]: risk, ltv = "A", CFG["LtvA"]
    elif score >= CFG["ConfModest"]: risk, ltv = "B", CFG["LtvB"]
    else: risk, ltv = "C", CFG["LtvC"]
    return dict(compCount=len(p), keptCount=len(kept), rejectedCount=len(p) - len(kept), fmvLow=max(point - half, 1),
                fmvPoint=point, fmvHigh=point + half, spreadBps=spread, confidenceScore=score,
                confidenceTier=tier, eligible=eligible, riskTier=risk, ltvBps=ltv)

def get(u): return urllib.request.urlopen(urllib.request.Request(u, headers=UA), timeout=60).read().decode()
def money(s): return float(s.replace("$", "").replace(",", ""))
def norm(g):
    m = re.match(r"\s*([A-Za-z]+).*?(\d+(?:\.\d)?)\s*$", g or "")
    return (m.group(1).upper(), m.group(2)) if m else (g, "")

def main():
    cc = {c["certId"]: c for c in json.load(open("seed_cards_collectorcrypt.json"))["cards"]}
    cards = []
    for cert, ids in MAP.items():
        s = cc[cert]
        num = re.search(r"(\d+(?:\.\d)?)\s*$", s["grade"]).group(1)
        comps = []
        for gid in ids:
            t = get(f"{BASE}/app/cards/{gid}/"); time.sleep(0.7)
            for href, body in re.findall(r'<a class="table-row" href="([^"]+)"[^>]*>(.*?)</a>', t, re.S):
                sm = re.search(r"<small>(.*?)</small>", body, re.S)
                src = html.unescape(sm.group(1)).strip() if sm else ""
                if "/" not in src: continue
                platform, grade = [x.strip() for x in src.split("/", 1)]
                if norm(grade) != (s["grader"].upper(), num): continue
                spans = [html.unescape(re.sub(r"<[^>]+>", "", x)).strip() for x in re.findall(r"<span[^>]*>(.*?)</span>", body, re.S)]
                try: ask, fmv = money(spans[-3]), money(spans[-2])
                except Exception: continue
                comps.append(dict(kind="gg_oracle_fmv", cents=int(round(fmv * 100)), askCents=int(round(ask * 100)),
                                  platform=platform, ggCardId=gid, url=href))
        a = appraise([x["cents"] for x in comps])
        ca = s["appraisal"]
        cards.append(dict(name=s["name"], grader=s["grader"], certId=cert, grade=s["grade"], subjectNft=s["subjectNft"],
                          solscan=s["solscan"], priceSource="gacha_galaxy_oracle", comps=comps, appraisal=a,
                          crossCheck=dict(source="collector_crypt", fmvLow=ca["fmvLow"], fmvPoint=ca["fmvPoint"], fmvHigh=ca["fmvHigh"],
                                          gapPct=round((a["fmvPoint"] - ca["fmvPoint"]) / ca["fmvPoint"] * 100, 1))))
        print(f"{s['name'][:50]:50} comps={len(comps):2} (kept {a['keptCount']}) ${a['fmvLow']/100:,.0f}-${a['fmvHigh']/100:,.0f} "
              f"pt ${a['fmvPoint']/100:,.0f} {a['confidenceTier']:9} LTV {a['ltvBps']//100:2}% {a['riskTier']:6} "
              f"max loan ${a['fmvLow']*a['ltvBps']/1e6:,.0f} | vs CC {cards[-1]['crossCheck']['gapPct']:+.1f}%")

    json.dump(dict(source="Gacha Galaxy oracle (gachagalaxy.io/app) — same-grade live listings; subject slabs from Collector Crypt public API",
                   priceBasis="Gacha Galaxy oracle fair values of live same-grade listings; NOT completed sales",
                   pulledAt=datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"), scoring=CFG, cards=cards),
              open("seed_cards.json", "w"), indent=2)
    print("wrote seed_cards.json (primary: Gacha Galaxy oracle)")


if __name__ == "__main__":
    main()
