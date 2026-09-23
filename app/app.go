package app

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/HorizenOfficial/vela-common-go/wasm/types"
	"github.com/HorizenOfficial/vela-common-go/wasm/utils"
	"github.com/gachagalaxyhq/gacha-vela-appraisal/internal/reqtype"
	"github.com/gachagalaxyhq/gacha-vela-appraisal/internal/subtype"
)

// Now is a var so tests can stub it. Matches the vela-nova pattern.
var Now = func() int64 { return time.Now().Unix() }

// TODO: make this configurable per-card rather than global
const defaultMaxComps = 50

func Deploy(appId int64, paramsJSON string) types.DeployResult {
	state := &ApplicationInternalState{
		AppID:       uint64(appId),
		Cards:       make(map[string]*CardProfile),
		Portfolios:  make(map[string]*Portfolio),
		DataSources: make(map[string]bool),
		MaxComps:    defaultMaxComps,
		Scoring:     defaultScoring(),
	}

	if paramsJSON != "" {
		var params DeployParams
		if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
			utils.LogError("Deploy: failed to parse params: %v", err)
			return types.DeployResult{Error: fmt.Sprintf("failed to parse deploy params: %v", err)}
		}
		for _, s := range params.DataSources {
			state.DataSources[s] = true
		}
		if params.MaxComps > 0 {
			state.MaxComps = params.MaxComps
		}
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		utils.LogError("Deploy: failed to marshal state: %v", err)
		return types.DeployResult{Error: fmt.Sprintf("failed to marshal initial state: %v", err)}
	}
	utils.LogDebug("Deploy: appId=%d sources=%d maxComps=%d", uint64(appId), len(state.DataSources), state.MaxComps)
	return types.DeployResult{State: stateJSON, Fuel: types.NewUint256(5)}
}

// retained for cache warm-up; new deploys use Deploy
func LoadModule(appId int64) types.LoadModuleResult {
	state := &ApplicationInternalState{
		AppID:       uint64(appId),
		Cards:       make(map[string]*CardProfile),
		Portfolios:  make(map[string]*Portfolio),
		DataSources: make(map[string]bool),
		MaxComps:    defaultMaxComps,
		Scoring:     defaultScoring(),
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return types.LoadModuleResult{Error: fmt.Sprintf("failed to marshal initial state: %v", err)}
	}
	return types.LoadModuleResult{State: stateJSON, Fuel: types.NewUint256(5)}
}

// no-op: non-custodial oracle, rejects all deposits
func DepositFunds(senderPtr *types.Address, tokenPtr *types.Address, value *types.Uint256, stateJSON string) types.DepositResult {
	return types.DepositResult{
		State: []byte(stateJSON),
		Error: "gacha-vela-appraisal does not accept deposits (non-custodial oracle)",
		Fuel:  types.NewUint256(1),
	}
}

func ProcessRequest(senderPtr *types.Address, requestType int32, payloadJSON, stateJSON string) types.ProcessResult {
	if senderPtr == nil {
		return types.ProcessResult{Error: "sender address is nil"}
	}
	sender := *senderPtr
	senderHex := sender.Hex()

	var state ApplicationInternalState
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return types.ProcessResult{Error: fmt.Sprintf("failed to parse application state: %v", err)}
	}
	if state.Cards == nil {
		state.Cards = make(map[string]*CardProfile)
	}
	if state.Portfolios == nil {
		state.Portfolios = make(map[string]*Portfolio)
	}
	if state.DataSources == nil {
		state.DataSources = make(map[string]bool)
	}
	if state.Scoring == nil {
		state.Scoring = defaultScoring()
	}

	var instructions PayloadInstructions
	if payloadJSON != "" && payloadJSON != "{}" {
		if err := json.Unmarshal([]byte(payloadJSON), &instructions); err != nil {
			return types.ProcessResult{Error: fmt.Sprintf("failed to parse payload instructions: %v", err)}
		}
	}

	// Deanonymize via requestType (authority flow)
	if requestType == int32(reqtype.Deanonymize) {
		return deanonymize(&state, &instructions, stateJSON)
	}

	var events []types.PlainEvent
	var appEvents []types.AppEvent

	switch instructions.Type {

	case "submit_comps":
		if instructions.SubmitComps == nil {
			return types.ProcessResult{Error: "submitComps instruction missing"}
		}
		if !state.DataSources[senderHex] {
			utils.LogError("ProcessRequest: unauthorized data source %s", senderHex)
			return types.ProcessResult{Error: "sender is not an authorized data source"}
		}
		accepted, rejected := submitComps(&state, instructions.SubmitComps)
		evt := CompsAcceptedEvent{Type: "comps_accepted", CertID: instructions.SubmitComps.CertID, Accepted: accepted, Rejected: rejected}
		b, err := json.Marshal(evt)
		if err != nil {
			return types.ProcessResult{Error: fmt.Sprintf("failed to serialize comps event: %v", err)}
		}
		events = append(events, types.PlainEvent{UserID: sender, EventSubType: subtype.FromString("comps"), Data: b})

	case "appraise":
		if instructions.Appraise == nil {
			return types.ProcessResult{Error: "appraise instruction missing"}
		}
		certID := instructions.Appraise.CertID
		card, ok := state.Cards[certID]
		if !ok {
			return types.ProcessResult{Error: fmt.Sprintf("card %s not found (no comps submitted)", certID)}
		}
		result := appraise(state.Scoring, card, Now())
		card.LastAppraisal = &result
		card.AppraisalCount++

		evt := AppraisalEvent{Type: "appraisal", Appraisal: result, Nonce: card.AppraisalCount}
		b, err := json.Marshal(evt)
		if err != nil {
			return types.ProcessResult{Error: fmt.Sprintf("failed to serialize appraisal event: %v", err)}
		}
		// private per-user event to the requester
		events = append(events, types.PlainEvent{UserID: sender, EventSubType: subtype.FromString("appraisal"), Data: b})
		// public app event: only the attested output, never raw comps
		pub, err := json.Marshal(result)
		if err != nil {
			return types.ProcessResult{Error: fmt.Sprintf("failed to serialize public appraisal: %v", err)}
		}
		appEvents = append(appEvents, types.AppEvent{EventSubType: subtype.FromString("appraisal"), Data: pub})

	case "pledge":
		if instructions.Pledge == nil {
			return types.ProcessResult{Error: "pledge instruction missing"}
		}
		port, ok := state.Portfolios[senderHex]
		if !ok {
			port = &Portfolio{Owner: sender, Entries: []PortfolioEntry{}}
			state.Portfolios[senderHex] = port
		}
		added := 0
		for _, cid := range instructions.Pledge.CertIDs {
			// only pledge cards that have a valid appraisal
			if card, ok := state.Cards[cid]; ok && card.LastAppraisal != nil {
				port.Entries = append(port.Entries, PortfolioEntry{CertID: cid})
				added++
			}
		}
		port.Nonce++

	case "assess":
		port, ok := state.Portfolios[senderHex]
		if !ok || len(port.Entries) == 0 {
			return types.ProcessResult{Error: "no pledged portfolio for sender"}
		}
		assess := assessPortfolio(&state, port)
		b, err := json.Marshal(assess)
		if err != nil {
			return types.ProcessResult{Error: fmt.Sprintf("failed to serialize assessment event: %v", err)}
		}
		events = append(events, types.PlainEvent{UserID: sender, EventSubType: subtype.FromString("assessment"), Data: b})
		// public app event: aggregate attested output only (no per-card raw data)
		appEvents = append(appEvents, types.AppEvent{EventSubType: subtype.FromString("assessment"), Data: b})

	case "add_source":
		if instructions.AddSource == nil {
			return types.ProcessResult{Error: "addSource instruction missing"}
		}
		if state.Admin != "" && senderHex != state.Admin {
			return types.ProcessResult{Error: "only admin may add data sources"}
		}
		state.DataSources[instructions.AddSource.Source] = true

	case "configure":
		if instructions.Configure == nil || instructions.Configure.Scoring == nil {
			return types.ProcessResult{Error: "configure instruction missing"}
		}
		if state.Admin != "" && senderHex != state.Admin {
			return types.ProcessResult{Error: "only admin may configure"}
		}
		state.Scoring = instructions.Configure.Scoring

	default:
		return types.ProcessResult{Error: fmt.Sprintf("unsupported instruction type: [%s]", instructions.Type)}
	}

	newState, err := json.Marshal(&state)
	if err != nil {
		return types.ProcessResult{Error: fmt.Sprintf("failed to serialize new state: %v", err)}
	}
	utils.LogDebug("ProcessRequest: sender=%s type=%s events=%d appEvents=%d", senderHex, instructions.Type, len(events), len(appEvents))
	return types.ProcessResult{
		State:     newState,
		Events:    events,
		AppEvents: appEvents,
		Fuel:      types.NewUint256(50),
	}
}

func submitComps(state *ApplicationInternalState, in *SubmitCompsInstruction) (int, int) {
	card, ok := state.Cards[in.CertID]
	if !ok {
		card = &CardProfile{CertID: in.CertID, Grader: in.Grader, Grade: in.Grade, Comps: []CompRecord{}}
		state.Cards[in.CertID] = card
	}
	accepted, rejected := 0, 0
	for _, c := range in.Comps {
		if c.Price == nil || c.Price.IsZero() {
			rejected++
			continue
		}
		card.Comps = append(card.Comps, c)
		accepted++
	}
	// ring-bound
	if len(card.Comps) > state.MaxComps {
		card.Comps = card.Comps[len(card.Comps)-state.MaxComps:]
	}
	return accepted, rejected
}

func assessPortfolio(state *ApplicationInternalState, port *Portfolio) AssessmentEvent {
	low := types.NewUint256(0)
	high := types.NewUint256(0)
	maxLoan := types.NewUint256(0)
	eligible := []string{}
	worstTier := "A"
	tierRank := map[string]int{"A": 0, "B": 1, "C": 2, "REJECT": 3}

	for _, e := range port.Entries {
		card, ok := state.Cards[e.CertID]
		if !ok || card.LastAppraisal == nil {
			continue
		}
		a := card.LastAppraisal
		lo := parseDec(a.FMVLow)
		hi := parseDec(a.FMVHigh)
		low.AddOverflow(*low, *lo)
		high.AddOverflow(*high, *hi)
		if a.Eligible {
			eligible = append(eligible, e.CertID)
			// max loan contribution = FMV point * LTV
			pt := parseDec(a.FMVPoint)
			ml := mulBps(pt, a.LTVBps)
			maxLoan.AddOverflow(*maxLoan, ml)
		}
		if tierRank[a.RiskTier] > tierRank[worstTier] {
			worstTier = a.RiskTier
		}
	}
	if len(eligible) == 0 {
		worstTier = "REJECT"
	}
	return AssessmentEvent{
		Type:          "assessment",
		Owner:         port.Owner.Hex(),
		TotalFMVLow:   low.String(),
		TotalFMVHigh:  high.String(),
		MaxLoanUSD:    maxLoan.String(),
		EligibleCards: eligible,
		RiskTier:      worstTier,
		Nonce:         port.Nonce,
	}
}

func parseDec(s string) *types.Uint256 {
	if s == "" {
		return types.NewUint256(0)
	}
	out := types.NewUint256(0)
	ten := types.NewUint256(10)
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			break
		}
		out = mulUint(out, ten)
		d := types.NewUint256(uint64(ch - '0'))
		out.AddOverflow(*out, *d)
	}
	return out
}

// authority flow: latest attested output per card only, no raw comps
func deanonymize(state *ApplicationInternalState, instructions *PayloadInstructions, stateJSON string) types.ProcessResult {
	report := DeanonymizationReport{
		CardCount:   len(state.Cards),
		Cards:       make(map[string]*AppraisalOut),
		DataSources: state.DataSources,
	}
	for id, card := range state.Cards {
		if card.LastAppraisal != nil {
			report.Cards[id] = card.LastAppraisal
		}
	}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		return types.ProcessResult{Error: fmt.Sprintf("failed to serialize report: %v", err)}
	}
	return types.ProcessResult{
		State:  []byte(stateJSON), // unchanged
		Report: reportBytes,
		Fuel:   types.NewUint256(20),
	}
}

func GetAllocatedMemoryStats() types.MemoryStats {
	mapSize, totalBytes := utils.GetAllocatedMemoryStats()
	return types.MemoryStats{MapSize: mapSize, CumulativeMemorySize: totalBytes}
}
