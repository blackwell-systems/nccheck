package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/blackwell-systems/nccheck/registry"
	"github.com/blackwell-systems/nccheck/verify"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: nccheck <registry.yaml>\n")
		os.Exit(1)
	}

	path := os.Args[1]
	start := time.Now()

	// Load and parse.
	reg, err := registry.LoadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	// Compile expressions.
	cr, err := verify.Compile(reg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "COMPILE ERROR: %v\n", err)
		os.Exit(1)
	}

	schema := cr.Schema

	// Print header.
	fmt.Printf("nccheck — Normalization Confluence Verifier\n")
	fmt.Printf("════════════════════════════════════════════\n\n")
	fmt.Printf("Registry:    %s\n", reg.Name)
	fmt.Printf("Source:      %s\n\n", path)

	// State space summary.
	var varParts []string
	for _, v := range schema.Vars {
		switch v.Type {
		case registry.TypeBool:
			varParts = append(varParts, fmt.Sprintf("%s:bool", v.Name))
		case registry.TypeEnum:
			varParts = append(varParts, fmt.Sprintf("%s:enum(%d)", v.Name, v.Size))
		case registry.TypeInt:
			varParts = append(varParts, fmt.Sprintf("%s:int[%d..%d]", v.Name, v.Min, v.Max))
		}
	}
	fmt.Printf("State Space\n")
	fmt.Printf("  Variables: %s\n", strings.Join(varParts, " × "))
	fmt.Printf("  Total:     %d states\n", schema.TotalLen)

	// Build tables.
	if err := cr.BuildTables(); err != nil {
		fmt.Fprintf(os.Stderr, "\nTABLE BUILD ERROR: %v\n", err)
		os.Exit(1)
	}

	validCount, invalidCount := cr.Stats()
	fmt.Printf("  Valid:     %d\n", validCount)
	fmt.Printf("  Invalid:   %d\n\n", invalidCount)

	// Events and invariants.
	fmt.Printf("Events:      %d", len(reg.Events))
	var evtNames []string
	for _, e := range reg.Events {
		evtNames = append(evtNames, e.Name)
	}
	fmt.Printf("  [%s]\n", strings.Join(evtNames, ", "))

	fmt.Printf("Invariants:  %d", len(reg.Invariants))
	var invNames []string
	for _, inv := range reg.Invariants {
		invNames = append(invNames, inv.Name)
	}
	fmt.Printf("  [%s]\n\n", strings.Join(invNames, ", "))

	// WFC check.
	fmt.Printf("WFC (Well-Founded Compensation)\n")
	wfcPass, maxDepth, badState, err := cr.CheckWFC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ERROR: %v\n", err)
		os.Exit(1)
	}
	if wfcPass {
		fmt.Printf("  Result:    PASS\n")
		fmt.Printf("  Max depth: %d\n\n", maxDepth)
	} else {
		fmt.Printf("  Result:    FAIL\n")
		fmt.Printf("  Failure:   %s\n\n", badState)
	}

	// CC check.
	fmt.Printf("CC (Compensation Commutativity)\n")
	ccResult := cr.CheckCC()

	if ccResult.CC1Pass {
		fmt.Printf("  CC1:       PASS  (%d independent pairs checked, %d dependent skipped)\n",
			ccResult.PairsChecked, ccResult.DependentSkipped)
	} else {
		fmt.Printf("  CC1:       FAIL\n")
		fmt.Printf("    Events:  (%s, %s)\n", ccResult.CC1FailEvent1, ccResult.CC1FailEvent2)
		fmt.Printf("    State:   %s\n", ccResult.CC1FailState)
		fmt.Printf("    Order 1: %s → %s → %s\n",
			ccResult.CC1FailEvent1, ccResult.CC1FailEvent2, ccResult.CC1FailNF1)
		fmt.Printf("    Order 2: %s → %s → %s\n",
			ccResult.CC1FailEvent2, ccResult.CC1FailEvent1, ccResult.CC1FailNF2)
	}

	if ccResult.CC2Pass {
		fmt.Printf("  CC2:       PASS\n")
	} else {
		fmt.Printf("  CC2:       FAIL\n")
		fmt.Printf("    Event:   %s\n", ccResult.CC2FailEvent)
		fmt.Printf("    State:   %s\n", ccResult.CC2FailState)
		fmt.Printf("    NF(s):   %s\n", ccResult.CC2FailNFState)
		fmt.Printf("    Step(e,s):     → %s\n", ccResult.CC2FailNF1)
		fmt.Printf("    Step(e,NF(s)): → %s\n", ccResult.CC2FailNF2)
	}
	fmt.Println()

	// Summary.
	elapsed := time.Since(start)
	allPass := wfcPass && ccResult.CCPass

	fmt.Printf("════════════════════════════════════════════\n")
	if allPass {
		fmt.Printf("Unique Normal Form:  YES\n")
		fmt.Printf("Convergence:         GUARANTEED\n")
	} else {
		fmt.Printf("Convergence:         NOT GUARANTEED\n")
		if !wfcPass {
			fmt.Printf("  ✗ WFC failed\n")
		}
		if !ccResult.CC1Pass {
			fmt.Printf("  ✗ CC1 failed\n")
		}
		if !ccResult.CC2Pass {
			fmt.Printf("  ✗ CC2 failed\n")
		}
	}
	fmt.Printf("Checked in:          %v\n", elapsed.Round(time.Microsecond))

	if !allPass {
		os.Exit(1)
	}
}
