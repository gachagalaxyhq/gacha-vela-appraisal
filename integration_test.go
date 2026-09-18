package main

// End-to-end integration test: loads the REAL compiled WASM module into a
// Wasmtime runtime (the same runtime the Vela Executor uses) and drives the
// full user flow through the exported guest functions, passing state in/out
// exactly as the host does.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/bytecodealliance/wasmtime-go/v3"
)

// wasmHost is a minimal host harness around the compiled module.
type wasmHost struct {
	store    *wasmtime.Store
	memory   *wasmtime.Memory
	alloc    *wasmtime.Func
	dealloc  *wasmtime.Func
	deploy   *wasmtime.Func
	process  *wasmtime.Func
}

func newWasmHost(t *testing.T, wasmPath string) *wasmHost {
	t.Helper()
	engine := wasmtime.NewEngine()
	module, err := wasmtime.NewModuleFromFile(engine, wasmPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	store := wasmtime.NewStore(engine)
	linker := wasmtime.NewLinker(engine)
	if err := linker.DefineWasi(); err != nil {
		t.Fatalf("define wasi: %v", err)
	}
	wasi := wasmtime.NewWasiConfig()
	store.SetWasi(wasi)
	instance, err := linker.Instantiate(store, module)
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	h := &wasmHost{store: store}
	h.memory = instance.GetExport(store, "memory").Memory()
	h.alloc = instance.GetExport(store, "allocate").Func()
	h.dealloc = instance.GetExport(store, "deallocate").Func()
	h.deploy = instance.GetExport(store, "deploy").Func()
	h.process = instance.GetExport(store, "process_request").Func()
	if h.memory == nil || h.alloc == nil || h.deploy == nil || h.process == nil {
		t.Fatalf("missing exports")
	}
	return h
}

// writeBytes copies b into wasm memory via allocate and returns (ptr, len).
func (h *wasmHost) writeBytes(t *testing.T, b []byte) (int32, int32) {
	t.Helper()
	res, err := h.alloc.Call(h.store, len(b))
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	ptr := res.(int32)
	copy(h.memory.UnsafeData(h.store)[ptr:ptr+int32(len(b))], b)
	return ptr, int32(len(b))
}

// readResult reads a length-prefixed JSON result returned by the guest.
// The guest's SerializeAndWriteResult returns a pointer to [len(4 LE)][json].
func (h *wasmHost) readResult(t *testing.T, ptr int32) []byte {
	t.Helper()
	data := h.memory.UnsafeData(h.store)
	l := int32(data[ptr]) | int32(data[ptr+1])<<8 | int32(data[ptr+2])<<16 | int32(data[ptr+3])<<24
	out := make([]byte, l)
	copy(out, data[ptr+4:ptr+4+l])
	return out
}

func addrBytes(t *testing.T, hex string) []byte {
	t.Helper()
	// strip 0x
	s := hex[2:]
	b := make([]byte, 20)
	for i := 0; i < 20; i++ {
		fmt.Sscanf(s[i*2:i*2+2], "%02x", &b[i])
	}
	return b
}

const (
	wDealer   = "0x1111111111111111111111111111111111111111"
	wCollector = "0x3333333333333333333333333333333333333333"
)

func TestWasmEndToEnd(t *testing.T) {
	if _, err := os.Stat("build/gacha_appraisal.wasm"); os.IsNotExist(err) {
		t.Skip("wasm not built; run tinygo build first")
	}
	h := newWasmHost(t, "build/gacha_appraisal.wasm")

	// 1) deploy
	params := `{"dataSources":["` + wDealer + `"],"maxComps":50}`
	pp, pl := h.writeBytes(t, []byte(params))
	resPtr, err := h.deploy.Call(h.store, 1, pp, pl)
	if err != nil {
		t.Fatalf("deploy call: %v", err)
	}
	deployRes := h.readResult(t, resPtr.(int32))
	var dr struct {
		State string `json:"state"` // base64-encoded []byte
		Error string `json:"error"`
	}
	if err := json.Unmarshal(deployRes, &dr); err != nil {
		t.Fatalf("deploy result parse: %v (%s)", err, string(deployRes))
	}
	if dr.Error != "" {
		t.Fatalf("deploy error: %s", dr.Error)
	}
	state, err := base64.StdEncoding.DecodeString(dr.State)
	if err != nil {
		t.Fatalf("state b64 decode: %v", err)
	}
	t.Logf("deploy ok, state %d bytes", len(state))

	// stateFromResult extracts the state field. The guest returns State as
	// []byte, which encoding/json encodes as a base64 STRING; decode it back.
	stateFromResult := func(r map[string]interface{}) []byte {
		s, _ := r["state"].(string)
		raw, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			// maybe it was already plain
			return []byte(s)
		}
		return raw
	}

	call := func(senderHex, payload string, state []byte) map[string]interface{} {
		t.Helper()
		sp, sl := h.writeBytes(t, addrBytes(t, senderHex))
		pp, pl := h.writeBytes(t, []byte(payload))
		stp, stl := h.writeBytes(t, state)
		rp, err := h.process.Call(h.store, 1, sp, sl, 1 /*Process*/, pp, pl, stp, stl)
		if err != nil {
			t.Fatalf("process call: %v", err)
		}
		raw := h.readResult(t, rp.(int32))
		var out map[string]interface{}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("process result parse: %v (%s)", err, string(raw))
		}
		return out
	}

	// 2) dealer submits comps (tight, ~$1000 = 100000 cents)
	submit := `{"type":"submit_comps","submitComps":{"certId":"PSA-999","grader":"PSA","grade":"10","comps":[` +
		`{"source":"dealer","price":"0x186a0","timestamp":1700000000},` +
		`{"source":"dealer","price":"0x186e6","timestamp":1700000000},` +
		`{"source":"dealer","price":"0x1863a","timestamp":1700000000},` +
		`{"source":"dealer","price":"0x186dc","timestamp":1700000000},` +
		`{"source":"dealer","price":"0x18614","timestamp":1700000000}]}}`
	r1 := call(wDealer, submit, state)
	if e, _ := r1["error"].(string); e != "" {
		t.Fatalf("submit_comps error: %s", e)
	}
	state = stateFromResult(r1)

	// 3) appraise -> attested output in appEvents
	r2 := call(wCollector, `{"type":"appraise","appraise":{"certId":"PSA-999"}}`, state)
	if e, _ := r2["error"].(string); e != "" {
		t.Fatalf("appraise error: %s", e)
	}
	state = stateFromResult(r2)
	appEvents, _ := r2["appEvents"].([]interface{})
	if len(appEvents) == 0 {
		t.Fatalf("expected public appraisal app event")
	}
	ae := appEvents[0].(map[string]interface{})
	aeData, _ := base64.StdEncoding.DecodeString(ae["data"].(string))
	var appraisal map[string]interface{}
	json.Unmarshal(aeData, &appraisal)
	t.Logf("ATTESTED APPRAISAL: %v", appraisal)
	if appraisal["confidenceTier"] != "REAL" {
		t.Fatalf("expected REAL, got %v", appraisal["confidenceTier"])
	}
	if appraisal["eligible"] != true {
		t.Fatalf("expected eligible=true")
	}

	// 4) pledge + assess
	r3 := call(wCollector, `{"type":"pledge","pledge":{"certIds":["PSA-999"]}}`, state)
	state = stateFromResult(r3)
	r4 := call(wCollector, `{"type":"assess","assess":{}}`, state)
	if e, _ := r4["error"].(string); e != "" {
		t.Fatalf("assess error: %s", e)
	}
	events, _ := r4["events"].([]interface{})
	if len(events) == 0 {
		t.Fatalf("expected assessment event")
	}
	ev := events[0].(map[string]interface{})
	evData, _ := base64.StdEncoding.DecodeString(ev["data"].(string))
	var assessment map[string]interface{}
	json.Unmarshal(evData, &assessment)
	t.Logf("COLLATERAL ASSESSMENT: %v", assessment)
	if assessment["maxLoanUsd"] == "0" || assessment["maxLoanUsd"] == nil {
		t.Fatalf("expected non-zero max loan, got %v", assessment["maxLoanUsd"])
	}

	fmt.Println("WASM END-TO-END: deploy -> submit_comps -> appraise -> pledge -> assess ALL OK")
}
