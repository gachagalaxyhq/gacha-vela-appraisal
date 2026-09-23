// Package reqtype mirrors the Vela host RequestType constants used by the
// guest to route process_request. Vendored from HorizenOfficial/vela/pkg/common
// so the WASM guest only depends on vela-common-go.
package reqtype

// RequestType represents the type of request being sent to the TEE.
type RequestType uint8

const (
	// Deploy represents a request to deploy a new application.
	Deploy RequestType = iota
	// Process is used for processing a batch of requests.
	Process
	// Deanonymize is used for deanonymization requests.
	Deanonymize
	// AssociateKey records an association between an address and a P521 pubkey.
	AssociateKey
	// TrustProcess is analogous to Process but its payload is sent in clear text.
	TrustProcess
)
