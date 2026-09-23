package app

import (
	"github.com/HorizenOfficial/vela-common-go/wasm/types"
)

type CompRecord struct {
	Source    string         `json:"source"`
	Price     *types.Uint256 `json:"price"` // USD cents
	Timestamp int64          `json:"timestamp"`
}

type CardProfile struct {
	CertID         string        `json:"certId"`
	Grader         string        `json:"grader"` // PSA | BGS | CGC | TAG | SGC
	Grade          string        `json:"grade"`
	Comps          []CompRecord  `json:"comps"`
	LastAppraisal  *AppraisalOut `json:"lastAppraisal,omitempty"`
	AppraisalCount uint64        `json:"appraisalCount"`
}

type AppraisalOut struct {
	CertID          string `json:"certId"`
	FMVLow          string `json:"fmvLow"`
	FMVHigh         string `json:"fmvHigh"`
	FMVPoint        string `json:"fmvPoint"`
	ConfidenceTier  string `json:"confidenceTier"`
	ConfidenceScore uint32 `json:"confidenceScore"`
	Eligible        bool   `json:"eligible"`
	LTVBps          uint32 `json:"ltvBps"`
	RiskTier        string `json:"riskTier"`
	CompCount       uint32 `json:"compCount"`
	SanityRejected  bool   `json:"sanityRejected"`
	Timestamp       int64  `json:"timestamp"`
}

type PortfolioEntry struct {
	CertID string `json:"certId"`
}

type Portfolio struct {
	Owner   types.Address    `json:"owner"`
	Entries []PortfolioEntry `json:"entries"`
	Nonce   uint64           `json:"nonce"`
}

// Scoring weights live in encrypted state, not the binary.
type ScoringConfig struct {
	MaxDevBps   uint32 `json:"maxDevBps"`
	ConfReal    uint32 `json:"confReal"`
	ConfModest  uint32 `json:"confModest"`
	MinEligible uint32 `json:"minEligible"`
	MinComps    int    `json:"minComps"`
	LtvA        uint32 `json:"ltvA"`
	LtvB        uint32 `json:"ltvB"`
	LtvC        uint32 `json:"ltvC"`
}

type ApplicationInternalState struct {
	AppID       uint64                  `json:"appId"`
	Cards       map[string]*CardProfile `json:"cards"`
	Portfolios  map[string]*Portfolio   `json:"portfolios"`
	DataSources map[string]bool         `json:"dataSources"`
	Admin       string                  `json:"admin"`
	MaxComps    int                     `json:"maxComps"`
	Scoring     *ScoringConfig          `json:"scoring"`
}

type DeployParams struct {
	DataSources []string `json:"dataSources"`
	MaxComps    int      `json:"maxComps"`
}

type SubmitCompsInstruction struct {
	CertID string       `json:"certId"`
	Grader string       `json:"grader"`
	Grade  string       `json:"grade"`
	Comps  []CompRecord `json:"comps"`
}

type AppraiseInstruction struct {
	CertID string `json:"certId"`
}

type PledgeInstruction struct {
	CertIDs []string `json:"certIds"`
}

type AssessInstruction struct{}

type AddSourceInstruction struct {
	Source string `json:"source"`
}

type ConfigureInstruction struct {
	Scoring *ScoringConfig `json:"scoring"`
}

type PayloadInstructions struct {
	Type        string                  `json:"type"`
	SubmitComps *SubmitCompsInstruction `json:"submitComps,omitempty"`
	Appraise    *AppraiseInstruction    `json:"appraise,omitempty"`
	Pledge      *PledgeInstruction      `json:"pledge,omitempty"`
	Assess      *AssessInstruction      `json:"assess,omitempty"`
	AddSource   *AddSourceInstruction   `json:"addSource,omitempty"`
	Configure   *ConfigureInstruction   `json:"configure,omitempty"`
}

type AppraisalEvent struct {
	Type      string       `json:"type"`
	Appraisal AppraisalOut `json:"appraisal"`
	Nonce     uint64       `json:"nonce"`
}

type AssessmentEvent struct {
	Type          string   `json:"type"`
	Owner         string   `json:"owner"`
	TotalFMVLow   string   `json:"totalFmvLow"`
	TotalFMVHigh  string   `json:"totalFmvHigh"`
	MaxLoanUSD    string   `json:"maxLoanUsd"`
	EligibleCards []string `json:"eligibleCards"`
	RiskTier      string   `json:"riskTier"`
	Nonce         uint64   `json:"nonce"`
}

type CompsAcceptedEvent struct {
	Type     string `json:"type"`
	CertID   string `json:"certId"`
	Accepted int    `json:"accepted"`
	Rejected int    `json:"rejected"`
}

type DeanonymizationReport struct {
	Tag         string                   `json:"tag,omitempty"`
	CardCount   int                      `json:"cardCount"`
	Cards       map[string]*AppraisalOut `json:"cards"`
	DataSources map[string]bool          `json:"dataSources"`
}
