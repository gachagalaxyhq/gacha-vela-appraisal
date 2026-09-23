package app

import (
	"math/bits"

	"github.com/HorizenOfficial/vela-common-go/wasm/types"
)

func defaultScoring() *ScoringConfig {
	return &ScoringConfig{
		MaxDevBps:  6000, // 60%
		ConfReal:   70,
		ConfModest: 40,
		MinEligible: 40,
		MinComps:   3,
		LtvA:       5000, // 50%
		LtvB:       3500,
		LtvC:       2000,
	}
}

func mul64(v *types.Uint256, m uint64) *types.Uint256 {
	out := &types.Uint256{}
	var carry uint64
	for i := 0; i < 4; i++ {
		hi, lo := bits.Mul64(v[i], m)
		lo, c := bits.Add64(lo, carry, 0)
		out[i] = lo
		carry = hi + c
	}
	return out
}

func mulUint(a, b *types.Uint256) *types.Uint256 {
	out := &types.Uint256{}
	for i := 0; i < 4; i++ {
		var carry uint64
		for j := 0; j+i < 4; j++ {
			hi, lo := bits.Mul64(a[j], b[i])
			sum, c1 := bits.Add64(lo, out[i+j], 0)
			sum, c2 := bits.Add64(sum, carry, 0)
			out[i+j] = sum
			carry = hi + c1 + c2
		}
	}
	return out
}

func divSmall(v *types.Uint256, d uint64) (*types.Uint256, uint64) {
	var q types.Uint256
	var rem uint64
	for i := 3; i >= 0; i-- {
		qq, rr := bits.Div64(rem, v[i], d)
		q[i] = qq
		rem = rr
	}
	return &q, rem
}

func halve(v *types.Uint256) *types.Uint256 {
	out := &types.Uint256{}
	out[3] = v[3] >> 1
	out[2] = (v[2] >> 1) | (v[3] << 63)
	out[1] = (v[1] >> 1) | (v[2] << 63)
	out[0] = (v[0] >> 1) | (v[1] << 63)
	return out
}

func mulBps(v *types.Uint256, bps uint32) types.Uint256 {
	prod := mul64(v, uint64(bps))
	q, _ := divSmall(prod, 10000)
	return *q
}

func divUint(a, b *types.Uint256) *types.Uint256 {
	if b.IsZero() {
		return types.NewUint256(0)
	}
	if b[1] == 0 && b[2] == 0 && b[3] == 0 {
		q, _ := divSmall(a, b[0])
		return q
	}
	rem := types.NewUint256(0)
	quot := types.NewUint256(0)
	for i := 255; i >= 0; i-- {
		rem = shl1(rem)
		if bitAt(a, i) {
			rem[0] |= 1
		}
		if rem.Cmp(*b) >= 0 {
			rem.Sub(*rem, *b)
			quot = setBit(quot, i)
		}
	}
	return quot
}

func shl1(v *types.Uint256) *types.Uint256 {
	out := &types.Uint256{}
	var carry uint64
	for i := 0; i < 4; i++ {
		out[i] = (v[i] << 1) | carry
		carry = v[i] >> 63
	}
	return out
}

func bitAt(v *types.Uint256, i int) bool {
	return (v[i/64]>>uint(i%64))&1 == 1
}

func setBit(v *types.Uint256, i int) *types.Uint256 {
	v[i/64] |= (1 << uint(i%64))
	return v
}

func medianU256(vals []types.Uint256) types.Uint256 {
	if len(vals) == 0 {
		return *types.NewUint256(0)
	}
	sorted := make([]types.Uint256, len(vals))
	copy(sorted, vals)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Cmp(sorted[j-1]) < 0; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	sum := types.NewUint256(0)
	sum.AddOverflow(sorted[mid-1], sorted[mid])
	return *halve(sum)
}

func relDevBps(p, ref *types.Uint256) uint32 {
	if ref.IsZero() {
		return 0
	}
	diff := types.NewUint256(0)
	if p.Cmp(*ref) >= 0 {
		diff.Sub(*p, *ref)
	} else {
		diff.Sub(*ref, *p)
	}
	q := divUint(mul64(diff, 10000), ref)
	return uint32(q[0])
}

func spreadBps(vals []types.Uint256, med *types.Uint256) uint32 {
	if len(vals) == 0 || med.IsZero() {
		return 0
	}
	min, max := vals[0], vals[0]
	for _, v := range vals {
		if v.Cmp(min) < 0 {
			min = v
		}
		if v.Cmp(max) > 0 {
			max = v
		}
	}
	diff := types.NewUint256(0)
	diff.Sub(max, min)
	q := divUint(mul64(diff, 10000), med)
	return uint32(q[0])
}

// TODO: revisit confidence model — currently linear dispersion penalty,
// probably want something non-linear once we have real comp data
func confScore(cfg *ScoringConfig, nComps int, spread uint32) uint32 {
	var base uint32
	switch {
	case nComps >= 10:
		base = 60
	case nComps >= 5:
		base = 45
	case nComps >= 3:
		base = 30
	case nComps >= 1:
		base = 15
	}
	penalty := spread * 40 / 6000
	if penalty > 40 {
		penalty = 40
	}
	score := int32(base) + 40 - int32(penalty)
	if nComps < 2 && score > int32(cfg.ConfModest)-1 {
		score = int32(cfg.ConfModest) - 1
	} else if nComps < 3 && score > int32(cfg.ConfReal)-1 {
		score = int32(cfg.ConfReal) - 1
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return uint32(score)
}

func tierFor(cfg *ScoringConfig, score uint32) string {
	if score >= cfg.ConfReal {
		return "REAL"
	}
	if score >= cfg.ConfModest {
		return "MODEST"
	}
	return "NARRATIVE"
}

func riskFor(cfg *ScoringConfig, score uint32, eligible bool) (string, uint32) {
	if !eligible {
		return "REJECT", 0
	}
	switch {
	case score >= cfg.ConfReal:
		return "A", cfg.LtvA
	case score >= cfg.ConfModest:
		return "B", cfg.LtvB
	default:
		return "C", cfg.LtvC
	}
}

func appraise(cfg *ScoringConfig, card *CardProfile, now int64) AppraisalOut {
	out := AppraisalOut{CertID: card.CertID, Timestamp: now}

	raw := make([]types.Uint256, 0, len(card.Comps))
	for _, c := range card.Comps {
		if c.Price != nil {
			raw = append(raw, *c.Price)
		}
	}
	out.CompCount = uint32(len(raw))
	if len(raw) == 0 {
		out.ConfidenceTier = "NARRATIVE"
		out.RiskTier = "REJECT"
		return out
	}

	med := medianU256(raw)
	kept := make([]types.Uint256, 0, len(raw))
	rejected := false
	for i := range raw {
		if relDevBps(&raw[i], &med) <= cfg.MaxDevBps {
			kept = append(kept, raw[i])
		} else {
			rejected = true
		}
	}
	out.SanityRejected = rejected
	if len(kept) == 0 {
		kept = raw
	}

	point := medianU256(kept)
	spread := spreadBps(kept, &point)
	out.FMVPoint = point.String()
	half := mulBps(&point, spread/2)
	low := types.NewUint256(0)
	if point.Cmp(half) > 0 {
		low.Sub(point, half)
	}
	high := types.NewUint256(0)
	high.AddOverflow(point, half)
	out.FMVLow = low.String()
	out.FMVHigh = high.String()

	score := confScore(cfg, len(kept), spread)
	out.ConfidenceScore = score
	out.ConfidenceTier = tierFor(cfg, score)

	eligible := score >= cfg.MinEligible && len(kept) >= cfg.MinComps
	out.Eligible = eligible
	tier, ltv := riskFor(cfg, score, eligible)
	out.RiskTier = tier
	out.LTVBps = ltv
	return out
}

