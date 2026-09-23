"""
Cross-check the Buildathon appraisals against Gacha Galaxy's OWN oracle (gachagalaxy.io).

Uses the live app's public pages/endpoints (no key):
  /app/cards/<id>/            -> per-listing "Market Depth": platform / grade, ask, oracle FMV
  /app/api/cards/<id>/snapshots/ -> 20-minute snapshot history (market_price, fmv, platform)

We keep only listings at the SAME grade as the certificate (PSA 10), then compare
Gacha Galaxy's oracle FMV with the appraisal band built from Collector Crypt data.
Price basis: the oracle FMV is Gacha Galaxy's own fair-value estimate; the ask is a live listing.
Neither is a completed sale.
"""
import re, html, json, time, statistics, urllib.request, datetime

UA = {"User-Agent": "Mozilla/5.0 gacha-buildathon"}
BASE = "https://www.gachagalaxy.io"
# card IDs in the Gacha Galaxy oracle for each seeded cert (several IDs = same card listed on different platforms)
MAP = {
    "109308847": [8922],               # Rayquaza VMAX #218 Evolving Skies
    "118973623": [6585],               # Pikachu & Zekrom GX #SM168
    "100863346": [6906],               # Lugia V #186 Silver Tempest
    "154790134": [798, 2638, 6472, 16085],  # Giratina VSTAR #GG69 Crown Zenith
    "153708045": [3492],               # Rayquaza VMAX #TG20 Silver Tempest
    "111992917": [11822, 2419],        # Umbreon VMAX #TG23 Brilliant Stars
}
def get(u):
    return urllib.request.urlopen(urllib.request.Request(u, headers=UA), timeout=60).read().decode()
def money(s): return float(s.replace("$", "").replace(",", ""))

seed = json.load(open("seed_cards.json"))
out = []
for c in seed["cards"]:
    rows = []
    for gid in MAP.get(c["certId"], []):
        t = get(f"{BASE}/app/cards/{gid}/"); time.sleep(0.7)
        for href, body in re.findall(r'<a class="table-row" href="([^"]+)"[^>]*>(.*?)</a>', t, re.S):
            spans = [html.unescape(re.sub(r"<[^>]+>", "", s)).strip() for s in re.findall(r"<span[^>]*>(.*?)</span>", body, re.S)]
            small = re.search(r"<small>(.*?)</small>", body, re.S)
            src = html.unescape(small.group(1)).strip() if small else ""
            if "/" not in src: continue
            platform, grade = [x.strip() for x in src.split("/", 1)]
            try: ask, fmv = money(spans[-3]), money(spans[-2])
            except Exception: continue
            rows.append(dict(ggCardId=gid, platform=platform, grade=grade, ask=ask, oracleFmv=fmv, url=href))
    def norm(g):  # "PSA GEM-MT 10" / "PSA 10" -> ("PSA", "10")
        m = re.match(r"\s*([A-Za-z]+).*?(\d+(?:\.\d)?)\s*$", g or "")
        return (m.group(1).upper(), m.group(2)) if m else (g, "")
    num = re.search(r"(\d+(?:\.\d)?)\s*$", c["grade"]).group(1)
    want = f"{c['grader']} {num}"
    same = [r for r in rows if norm(r["grade"]) == (c["grader"].upper(), num)]
    a = c["appraisal"]; lo, hi, pt = a["fmvLow"] / 100, a["fmvHigh"] / 100, a["fmvPoint"] / 100
    ofmv = [r["oracleFmv"] for r in same]
    med = statistics.median(ofmv) if ofmv else None
    rec = dict(certId=c["certId"], name=c["name"], grade=want, appraisal=dict(low=lo, point=pt, high=hi),
               ggListingsSameGrade=len(same), ggOracleFmvMedian=med,
               ggOracleFmvRange=[min(ofmv), max(ofmv)] if ofmv else None,
               ggPlatforms=sorted(set(r["platform"] for r in same)),
               deltaVsPointPct=round((med - pt) / pt * 100, 1) if med else None,
               withinBand=(lo <= med <= hi) if med else None, listings=same)
    out.append(rec)
    print(f"{c['name'][:48]:48} appraisal ${lo:,.0f}-${hi:,.0f} | GG oracle PSA10 n={len(same)} "
          f"median ${med:,.0f} " if med else f"{c['name'][:48]:48} no same-grade GG listings", 
          f"({rec['deltaVsPointPct']:+.1f}% vs point, {'IN band' if rec['withinBand'] else 'outside band'}) {rec['ggPlatforms']}" if med else "")
json.dump(dict(source="Gacha Galaxy oracle (gachagalaxy.io/app, public pages)", pulledAt=datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
               basis="Gacha Galaxy oracle FMV per live listing, same grade only; not completed sales", cards=out),
          open("gg_crosscheck.json", "w"), indent=2)
print("wrote gg_crosscheck.json")
