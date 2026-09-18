package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/HorizenOfficial/vela-common-go/wasm/types"
	"github.com/gachagalaxyhq/gacha-vela-appraisal/internal/reqtype"
)

// helper: USD cents Uint256
func cents(v uint64) *types.Uint256 { return types.NewUint256(v) }

func addr(hex string) *types.Address {
	a, err := types.HexToAddress(hex)
	if err != nil {
		panic(err)
	}
	return &a
}

const (
	srcDealer   = "0x1111111111111111111111111111111111111111"
	srcMarket   = "0x2222222222222222222222222222222222222222"
	collector   = "0x3333333333333333333333333333333333333333"
	unauthorized = "0x4444444444444444444444444444444444444444"
)

func deployState(t *testing.T) string {
	t.Helper()
	params := DeployParams{DataSources: []string{srcDealer, srcMarket}, MaxComps: 50}
	pj, _ := json.Marshal(params)
	res := Deploy(1, string(pj))
	if res.Error != "" {
		t.Fatalf("deploy error: %s", res.Error)
	}
	return string(res.State)
}

func submitCompsPayload(certID string, prices ...uint64) string {
	comps := []CompRecord{}
	for _, p := range prices {
		comps = append(comps, CompRecord{Source: "dealer", Price: cents(p), Timestamp: 1700000000})
	}
	in := PayloadInstructions{Type: "submit_comps", SubmitComps: &SubmitCompsInstruction{
		CertID: certID, Grader: "PSA", Grade: "10", Comps: comps,
	}}
	b, _ := json.Marshal(in)
	return string(b)
}

func appraisePayload(certID string) string {
	in := PayloadInstructions{Type: "appraise", Appraise: &AppraiseInstruction{CertID: certID}}
	b, _ := json.Marshal(in)
	return string(b)
}

func TestDeployInitializes(t *testing.T) {
	s := deployState(t)
	var st ApplicationInternalState
	if err := json.Unmarshal([]byte(s), &st); err != nil {
		t.Fatalf("state unmarshal: %v", err)
	}
	if !st.DataSources[srcDealer] || !st.DataSources[srcMarket] {
		t.Fatalf("expected data sources seeded")
	}
	if st.MaxComps != 50 {
		t.Fatalf("expected maxComps 50, got %d", st.MaxComps)
	}
}

func TestSubmitCompsUnauthorizedRejected(t *testing.T) {
	s := deployState(t)
	res := ProcessRequest(addr(unauthorized), int32(reqtype.Process), submitCompsPayload("PSA-12345"), s)
	if res.Error == "" || !strings.Contains(res.Error, "not an authorized") {
		t.Fatalf("expected unauthorized error, got %+v", res)
	}
}

func TestAppraisalRealTier(t *testing.T) {
	s := deployState(t)
	// dealer submits 12 tight comps around $1000 (100000 cents)
	res := ProcessRequest(addr(srcDealer), int32(reqtype.Process),
		submitCompsPayload("PSA-999", 98000, 99000, 100000, 101000, 102000, 100500, 99500, 100200, 99800, 100100, 100400, 100300), s)
	if res.Error != "" {
		t.Fatalf("submitComps error: %s", res.Error)
	}
	s = string(res.State)

	// appraise
	ar := ProcessRequest(addr(collector), int32(reqtype.Process), appraisePayload("PSA-999"), s)
	if ar.Error != "" {
		t.Fatalf("appraise error: %s", ar.Error)
	}
	// read the public app event (attested output)
	if len(ar.AppEvents) == 0 {
		t.Fatalf("expected a public appraisal app event")
	}
	var out AppraisalOut
	if err := json.Unmarshal(ar.AppEvents[0].Data, &out); err != nil {
		t.Fatalf("unmarshal appraisal: %v", err)
	}
	t.Logf("appraisal: %+v", out)
	if out.ConfidenceTier != "REAL" {
		t.Fatalf("expected REAL confidence for 12 tight comps, got %s (score %d)", out.ConfidenceTier, out.ConfidenceScore)
	}
	if !out.Eligible {
		t.Fatalf("expected eligible=true for REAL tier")
	}
	if out.RiskTier != "A" || out.LTVBps != defaultScoring().LtvA {
		t.Fatalf("expected tier A / LTV %d, got %s / %d", defaultScoring().LtvA, out.RiskTier, out.LTVBps)
	}
	// FMV point should be near 100000
	if out.FMVPoint == "" {
		t.Fatalf("empty FMV point")
	}
}

func TestSanityGateRejectsOutlier(t *testing.T) {
	s := deployState(t)
	// 5 comps near $500, one wild outlier at $100000 (200x) — should be rejected
	res := ProcessRequest(addr(srcMarket), int32(reqtype.Process),
		submitCompsPayload("CGC-777", 50000, 50100, 49900, 50050, 49950, 10000000), s)
	if res.Error != "" {
		t.Fatalf("submitComps error: %s", res.Error)
	}
	s = string(res.State)
	ar := ProcessRequest(addr(collector), int32(reqtype.Process), appraisePayload("CGC-777"), s)
	var out AppraisalOut
	json.Unmarshal(ar.AppEvents[0].Data, &out)
	t.Logf("sanity appraisal: %+v", out)
	if !out.SanityRejected {
		t.Fatalf("expected sanityRejected=true for the outlier comp")
	}
	// point should stay near 50000, not dragged by outlier
	pt := parseDec(out.FMVPoint)
	if pt.Cmp(*cents(60000)) > 0 {
		t.Fatalf("outlier dragged FMV: point=%s", out.FMVPoint)
	}
}

func TestSparseCompsNarrativeReject(t *testing.T) {
	s := deployState(t)
	// only 1 comp — below eligibleMinComps => NARRATIVE + REJECT
	res := ProcessRequest(addr(srcDealer), int32(reqtype.Process),
		submitCompsPayload("BGS-001", 250000), s)
	s = string(res.State)
	ar := ProcessRequest(addr(collector), int32(reqtype.Process), appraisePayload("BGS-001"), s)
	var out AppraisalOut
	json.Unmarshal(ar.AppEvents[0].Data, &out)
	t.Logf("sparse appraisal: %+v", out)
	if out.ConfidenceTier != "NARRATIVE" {
		t.Fatalf("expected NARRATIVE for 1 comp, got %s", out.ConfidenceTier)
	}
	if out.Eligible {
		t.Fatalf("expected eligible=false for sparse comps")
	}
	if out.RiskTier != "REJECT" {
		t.Fatalf("expected REJECT, got %s", out.RiskTier)
	}
}

func TestPledgeAndAssess(t *testing.T) {
	s := deployState(t)
	// two cards with good comps
	for _, cid := range []string{"PSA-A", "PSA-B"} {
		res := ProcessRequest(addr(srcDealer), int32(reqtype.Process),
			submitCompsPayload(cid, 100000, 101000, 99000, 100500, 99500), s)
		s = string(res.State)
		ar := ProcessRequest(addr(collector), int32(reqtype.Process), appraisePayload(cid), s)
		if ar.Error != "" {
			t.Fatalf("appraise %s: %s", cid, ar.Error)
		}
		s = string(ar.State)
	}
	// collector pledges both
	pledge := PayloadInstructions{Type: "pledge", Pledge: &PledgeInstruction{CertIDs: []string{"PSA-A", "PSA-B"}}}
	pb, _ := json.Marshal(pledge)
	pr := ProcessRequest(addr(collector), int32(reqtype.Process), string(pb), s)
	if pr.Error != "" {
		t.Fatalf("pledge: %s", pr.Error)
	}
	s = string(pr.State)
	// assess
	assess := PayloadInstructions{Type: "assess", Assess: &AssessInstruction{}}
	ab, _ := json.Marshal(assess)
	ar2 := ProcessRequest(addr(collector), int32(reqtype.Process), string(ab), s)
	if ar2.Error != "" {
		t.Fatalf("assess: %s", ar2.Error)
	}
	if len(ar2.Events) == 0 {
		t.Fatalf("expected assessment event")
	}
	var ev AssessmentEvent
	json.Unmarshal(ar2.Events[0].Data, &ev)
	t.Logf("assessment: %+v", ev)
	if len(ev.EligibleCards) != 2 {
		t.Fatalf("expected 2 eligible cards, got %d", len(ev.EligibleCards))
	}
	if ev.MaxLoanUSD == "0" || ev.MaxLoanUSD == "" {
		t.Fatalf("expected non-zero max loan")
	}
}

func TestCompsNeverLeakInPublicEvents(t *testing.T) {
	s := deployState(t)
	res := ProcessRequest(addr(srcDealer), int32(reqtype.Process),
		submitCompsPayload("PSA-SEC", 100000, 101000, 99000, 100500, 99500), s)
	s = string(res.State)
	ar := ProcessRequest(addr(collector), int32(reqtype.Process), appraisePayload("PSA-SEC"), s)
	// public app event must NOT contain any raw comp price
	pub := string(ar.AppEvents[0].Data)
	if strings.Contains(pub, "\"comps\"") || strings.Contains(pub, "\"source\":\"dealer\"") {
		t.Fatalf("raw comps leaked into public event: %s", pub)
	}
	// deanonymization report also must not contain raw comps
	dr := ProcessRequest(addr(collector), int32(reqtype.Deanonymize), "{}", s)
	if dr.Report == nil {
		t.Fatalf("expected deanonymization report")
	}
	if strings.Contains(string(dr.Report), "\"comps\"") {
		t.Fatalf("raw comps leaked into report: %s", string(dr.Report))
	}
	t.Logf("report (no raw comps): %s", string(dr.Report))
}
