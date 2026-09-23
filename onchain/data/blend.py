"""
Blend comps from both sources into ONE appraisal per slab:
  - Gacha Galaxy oracle fair values (same-grade live listings)   -> seed_cards_gg.json
  - Collector Crypt insured values + live asks (same item/grade) -> seed_cards_collectorcrypt.json
PokeTrace sold comps get added here once a key is available.
Same Gacha Galaxy appraisal model (median, 60% sanity filter, confidence, tier, LTV).
Price basis: none of these are completed sales.
"""
import json, datetime
from build_seed_gg import appraise, CFG

cc = {c["certId"]: c for c in json.load(open("seed_cards_collectorcrypt.json"))["cards"]}
gg = {c["certId"]: c for c in json.load(open("seed_cards_gg.json"))["cards"]}
cards = []
for cert, c in cc.items():
    comps = [dict(x, source="collector_crypt") for x in c["comps"]]
    comps += [dict(x, source="gacha_galaxy") for x in gg.get(cert, {}).get("comps", [])]
    a = appraise([x["cents"] for x in comps])
    n_gg = sum(x["source"] == "gacha_galaxy" for x in comps)
    cards.append(dict(name=c["name"], grader=c["grader"], certId=cert, grade=c["grade"], subjectNft=c["subjectNft"],
                      solscan=c["solscan"], sources=dict(collector_crypt=len(comps) - n_gg, gacha_galaxy=n_gg, poketrace=0),
                      comps=comps, appraisal=a))
    print(f"{c['name'][:48]:48} CC={len(comps)-n_gg:2} GG={n_gg:2} -> ${a['fmvLow']/100:,.0f}-${a['fmvHigh']/100:,.0f} "
          f"pt ${a['fmvPoint']/100:,.0f} {a['confidenceTier']:6} LTV {a['ltvBps']//100}% max loan ${a['fmvLow']*a['ltvBps']/1e6:,.0f}")
json.dump(dict(source="BLEND: Gacha Galaxy oracle + Collector Crypt public API (PokeTrace pending key)",
               priceBasis="oracle fair values, insured values and live asks of same-grade slabs; NOT completed sales",
               pulledAt=datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"), scoring=CFG, cards=cards),
          open("seed_cards.json", "w"), indent=2)
print("wrote blended seed_cards.json")
