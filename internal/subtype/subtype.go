// Package subtype packs a human-readable event name into a bytes32 event
// subtype. Vendored from HorizenOfficial/vela/app/subtype so the WASM guest
// does not import the (host-side) vela module, which is far too heavy to
// compile under TinyGo in constrained environments.
package subtype

// FromString packs up to 32 bytes of s, left-aligned and zero-padded, into a
// [32]byte suitable for use as a bytes32 event subtype. Bytes beyond 32 are
// silently dropped, so callers must ensure s fits.
func FromString(s string) [32]byte {
	var b [32]byte
	copy(b[:], s)
	return b
}
