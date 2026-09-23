// vela2rh runs the REAL Vela appraisal engine (app package, same code compiled into the
// enclave WASM) on comps from a JSON file, and prints the PUBLIC attested appraisal event
// that the enclave emits. Pipe the output into bridge/vela_to_registry.py.
//
//   go run ./cmd/vela2rh -comps onchain/data/seed_cards.json -cert 109308847 > appraisal.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/HorizenOfficial/vela-common-go/wasm/types"
	"github.com/gachagalaxyhq/gacha-vela-appraisal/app"
	"github.com/gachagalaxyhq/gacha-vela-appraisal/internal/reqtype"
)

type seed struct {
	Cards []struct {
		Grader string `json:"grader"`
		CertID string `json:"certId"`
		Grade  string `json:"grade"`
		Comps  []struct {
			Cents  uint64 `json:"cents"`
			Source string `json:"source"`
		} `json:"comps"`
	} `json:"cards"`
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func main() {
	compsPath := flag.String("comps", "onchain/data/seed_cards.json", "seed file with comps")
	cert := flag.String("cert", "", "cert id to appraise")
	flag.Parse()

	raw, err := os.ReadFile(*compsPath)
	must(err)
	var s seed
	must(json.Unmarshal(raw, &s))

	const src = "0x1111111111111111111111111111111111111111"
	pj, _ := json.Marshal(app.DeployParams{DataSources: []string{src}, MaxComps: 200})
	dep := app.Deploy(1, string(pj))
	if dep.Error != "" {
		must(fmt.Errorf("deploy: %s", dep.Error))
	}
	state := string(dep.State)
	sender, err := types.HexToAddress(src)
	must(err)

	for _, c := range s.Cards {
		if c.CertID != *cert {
			continue
		}
		comps := []app.CompRecord{}
		for _, x := range c.Comps {
			comps = append(comps, app.CompRecord{Source: x.Source, Price: types.NewUint256(x.Cents), Timestamp: 1790000000})
		}
		sub, _ := json.Marshal(app.PayloadInstructions{Type: "submit_comps", SubmitComps: &app.SubmitCompsInstruction{
			CertID: c.CertID, Grader: c.Grader, Grade: c.Grade, Comps: comps}})
		r := app.ProcessRequest(&sender, int32(reqtype.Process), string(sub), state)
		if r.Error != "" {
			must(fmt.Errorf("submit_comps: %s", r.Error))
		}
		state = string(r.State)

		ap, _ := json.Marshal(app.PayloadInstructions{Type: "appraise", Appraise: &app.AppraiseInstruction{CertID: c.CertID}})
		r = app.ProcessRequest(&sender, int32(reqtype.Process), string(ap), state)
		if r.Error != "" {
			must(fmt.Errorf("appraise: %s", r.Error))
		}
		for _, e := range r.AppEvents { // PUBLIC app events only: attested output, no comps
			os.Stdout.Write(e.Data)
			fmt.Println()
			return
		}
		must(fmt.Errorf("no public appraisal event"))
	}
	must(fmt.Errorf("cert %s not found", *cert))
}
