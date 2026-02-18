package verify

import (
	"fmt"
	"strings"

	"github.com/blackwell-systems/nccheck/expr"
	"github.com/blackwell-systems/nccheck/registry"
)

// CompiledRegistry holds precompiled expressions and lookup tables.
type CompiledRegistry struct {
	Reg          *registry.Registry
	Schema       registry.Schema
	EnumLiterals map[string]int

	InvExprs []*expr.Node // parsed invariant expressions
	RepExprs []map[int]*expr.Node // repair[i] -> varIdx -> parsed expr
	EvtGuards []*expr.Node // nil if no guard
	EvtExprs  []map[int]*expr.Node // event[i] -> varIdx -> parsed expr

	// Precomputed tables.
	Valid []bool                // Valid[stateID] = V(state)
	NF    []registry.StateID   // NF[stateID] = normal form
	Step  [][]registry.StateID // Step[eventIdx][stateID] = NF(apply(e, state))
	// -1 in Step means event not enabled at that state.
}

// Result holds verification results.
type Result struct {
	StateCount int
	VarSummary string

	WFCPass       bool
	WFCMaxDepth   int
	WFCError      string
	WFCBadState   string

	CCPass        bool
	CC1Pass       bool
	CC2Pass       bool
	CC1Error      string
	CC2Error      string
	CC1Pair       [2]string // event names
	CC1State      string
	CC1Trace1     string
	CC1Trace2     string

	EventCount    int
	InvariantCount int
	PairsChecked  int
	ValidStates   int
	InvalidStates int
}

const MaxStates = 1_000_000
const MaxRepairIter = 1000

// Compile parses all expressions and builds the compiled registry.
func Compile(reg *registry.Registry) (*CompiledRegistry, error) {
	schema := registry.NewSchema(reg.Vars)
	if schema.TotalLen > MaxStates {
		return nil, fmt.Errorf("state space too large: %d (max %d)", schema.TotalLen, MaxStates)
	}

	enumLiterals, err := expr.BuildEnumLiterals(&schema)
	if err != nil {
		return nil, err
	}

	cr := &CompiledRegistry{
		Reg:          reg,
		Schema:       schema,
		EnumLiterals: enumLiterals,
	}

	// Parse invariant expressions.
	for _, inv := range reg.Invariants {
		node, err := expr.Parse(inv.Expr)
		if err != nil {
			return nil, fmt.Errorf("invariant %q: %w", inv.Name, err)
		}
		cr.InvExprs = append(cr.InvExprs, node)
	}

	// Parse repair expressions.
	for _, rep := range reg.Compensation {
		repMap := make(map[int]*expr.Node)
		for varName, exprStr := range rep.Assignments {
			idx := schema.VarIndex(varName)
			if idx < 0 {
				return nil, fmt.Errorf("repair for %q: unknown variable %q", rep.Invariant, varName)
			}
			node, err := expr.Parse(exprStr)
			if err != nil {
				return nil, fmt.Errorf("repair for %q, var %q: %w", rep.Invariant, varName, err)
			}
			repMap[idx] = node
		}
		cr.RepExprs = append(cr.RepExprs, repMap)
	}

	// Parse event expressions.
	for _, evt := range reg.Events {
		var guard *expr.Node
		if evt.Guard != "" {
			guard, err = expr.Parse(evt.Guard)
			if err != nil {
				return nil, fmt.Errorf("event %q guard: %w", evt.Name, err)
			}
		}
		cr.EvtGuards = append(cr.EvtGuards, guard)

		evtMap := make(map[int]*expr.Node)
		for varName, exprStr := range evt.Assignments {
			idx := schema.VarIndex(varName)
			if idx < 0 {
				return nil, fmt.Errorf("event %q: unknown variable %q", evt.Name, varName)
			}
			node, err := expr.Parse(exprStr)
			if err != nil {
				return nil, fmt.Errorf("event %q, var %q: %w", evt.Name, varName, err)
			}
			evtMap[idx] = node
		}
		cr.EvtExprs = append(cr.EvtExprs, evtMap)
	}

	return cr, nil
}

// BuildTables precomputes Valid, NF, and Step tables.
func (cr *CompiledRegistry) BuildTables() error {
	n := cr.Schema.TotalLen
	cr.Valid = make([]bool, n)
	cr.NF = make([]registry.StateID, n)

	// 1. Compute Valid[s] for all states.
	for sid := 0; sid < n; sid++ {
		st := cr.Schema.Decode(registry.StateID(sid))
		v, err := cr.evalValid(st)
		if err != nil {
			return fmt.Errorf("validity check at state %s: %w", cr.fmtState(st), err)
		}
		cr.Valid[sid] = v
	}

	// 2. Compute NF[s] for all states.
	for sid := 0; sid < n; sid++ {
		nf, err := cr.computeNF(registry.StateID(sid))
		if err != nil {
			return fmt.Errorf("normal form at state %s: %w",
				cr.fmtState(cr.Schema.Decode(registry.StateID(sid))), err)
		}
		cr.NF[sid] = nf
	}

	// 3. Compute Step[e][s] for all events and states.
	cr.Step = make([][]registry.StateID, len(cr.Reg.Events))
	for ei := range cr.Reg.Events {
		cr.Step[ei] = make([]registry.StateID, n)
		for sid := 0; sid < n; sid++ {
			st := cr.Schema.Decode(registry.StateID(sid))
			enabled, err := cr.evalGuard(ei, st)
			if err != nil {
				return fmt.Errorf("event %q guard at state %s: %w",
					cr.Reg.Events[ei].Name, cr.fmtState(st), err)
			}
			if !enabled {
				cr.Step[ei][sid] = -1
				continue
			}
			post, err := cr.applyEvent(ei, st)
			if err != nil {
				return fmt.Errorf("event %q at state %s: %w",
					cr.Reg.Events[ei].Name, cr.fmtState(st), err)
			}
			postID := cr.Schema.Encode(post)
			cr.Step[ei][sid] = cr.NF[postID]
		}
	}

	return nil
}

// CheckWFC verifies well-founded compensation.
func (cr *CompiledRegistry) CheckWFC() (pass bool, maxDepth int, badState string, err error) {
	maxDepth = 0
	for sid := 0; sid < cr.Schema.TotalLen; sid++ {
		// Check that NF exists and is valid.
		nfID := cr.NF[sid]
		if !cr.Valid[nfID] {
			st := cr.Schema.Decode(registry.StateID(sid))
			nfSt := cr.Schema.Decode(nfID)
			return false, 0, fmt.Sprintf(
				"state %s → NF %s which is not valid",
				cr.fmtState(st), cr.fmtState(nfSt)), nil
		}
		// Check fixpoint: valid states are fixed.
		if cr.Valid[sid] && cr.NF[sid] != registry.StateID(sid) {
			st := cr.Schema.Decode(registry.StateID(sid))
			nfSt := cr.Schema.Decode(cr.NF[sid])
			return false, 0, fmt.Sprintf(
				"valid state %s has NF %s (not a fixpoint)",
				cr.fmtState(st), cr.fmtState(nfSt)), nil
		}
	}

	// Compute max depth from repair iteration counts.
	for sid := 0; sid < cr.Schema.TotalLen; sid++ {
		depth, err := cr.repairDepth(registry.StateID(sid))
		if err != nil {
			return false, 0, "", err
		}
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	return true, maxDepth, "", nil
}

// CheckCC checks compensation commutativity (CC1 and CC2).
func (cr *CompiledRegistry) CheckCC() (result CCResult) {
	n := cr.Schema.TotalLen
	numEvts := len(cr.Reg.Events)

	// Compute write sets and read sets for independence analysis.
	type evtSets struct {
		writes map[int]bool // var indices written
		reads  map[int]bool // var indices read (in guard + effect RHS)
	}
	sets := make([]evtSets, numEvts)
	for ei, evt := range cr.Reg.Events {
		s := evtSets{writes: map[int]bool{}, reads: map[int]bool{}}
		for varIdx := range cr.EvtExprs[ei] {
			s.writes[varIdx] = true
		}
		// Read sets: variables referenced in guard and effect expressions.
		if evt.Guard != "" {
			for _, v := range cr.Schema.Vars {
				idx := cr.Schema.VarIndex(v.Name)
				// Simple conservative approach: scan expression string for var names.
				if containsIdent(evt.Guard, v.Name) {
					s.reads[idx] = true
				}
			}
		}
		for _, exprStr := range evt.Assignments {
			for _, v := range cr.Schema.Vars {
				idx := cr.Schema.VarIndex(v.Name)
				if containsIdent(exprStr, v.Name) {
					s.reads[idx] = true
				}
			}
		}
		sets[ei] = s
	}

	// Two events are independent candidates if their write sets don't
	// intersect each other's read/write sets.
	isIndependent := func(e1, e2 int) bool {
		for w := range sets[e1].writes {
			if sets[e2].writes[w] || sets[e2].reads[w] {
				return false
			}
		}
		for w := range sets[e2].writes {
			if sets[e1].writes[w] || sets[e1].reads[w] {
				return false
			}
		}
		return true
	}

	// CC1: for independent event pairs (e1, e2), for all states s where both enabled:
	//   Step[e2][Step[e1][s]] == Step[e1][Step[e2][s]]
	result.CC1Pass = true
	for e1 := 0; e1 < numEvts && result.CC1Pass; e1++ {
		for e2 := e1 + 1; e2 < numEvts && result.CC1Pass; e2++ {
			if !isIndependent(e1, e2) {
				result.DependentSkipped++
				continue
			}
			result.PairsChecked++
			for sid := 0; sid < n; sid++ {
				s1 := cr.Step[e1][sid]
				s2 := cr.Step[e2][sid]
				if s1 == -1 || s2 == -1 {
					continue // at least one not enabled
				}

				// e1 then e2
				r12 := cr.Step[e2][s1]
				// e2 then e1
				r21 := cr.Step[e1][s2]

				// If either step is disabled in the intermediate state, skip.
				if r12 == -1 || r21 == -1 {
					continue
				}

				if r12 != r21 {
					result.CC1Pass = false
					st := cr.Schema.Decode(registry.StateID(sid))
					result.CC1FailEvent1 = cr.Reg.Events[e1].Name
					result.CC1FailEvent2 = cr.Reg.Events[e2].Name
					result.CC1FailState = cr.fmtState(st)
					result.CC1FailNF1 = cr.fmtState(cr.Schema.Decode(r12))
					result.CC1FailNF2 = cr.fmtState(cr.Schema.Decode(r21))
					break
				}
			}
		}
	}

	// CC2: for all events e, for all states s:
	//   Step[e][s] == Step[e][NF[s]]   (when both defined)
	result.CC2Pass = true
	for ei := 0; ei < numEvts && result.CC2Pass; ei++ {
		for sid := 0; sid < n; sid++ {
			stepRaw := cr.Step[ei][sid]
			if stepRaw == -1 {
				continue
			}
			nfID := cr.NF[sid]
			stepNF := cr.Step[ei][nfID]
			if stepNF == -1 {
				continue
			}
			if stepRaw != stepNF {
				result.CC2Pass = false
				st := cr.Schema.Decode(registry.StateID(sid))
				nfSt := cr.Schema.Decode(nfID)
				result.CC2FailEvent = cr.Reg.Events[ei].Name
				result.CC2FailState = cr.fmtState(st)
				result.CC2FailNFState = cr.fmtState(nfSt)
				result.CC2FailNF1 = cr.fmtState(cr.Schema.Decode(stepRaw))
				result.CC2FailNF2 = cr.fmtState(cr.Schema.Decode(stepNF))
				break
			}
		}
	}

	result.CCPass = result.CC1Pass && result.CC2Pass
	return
}

// CCResult holds CC verification results.
type CCResult struct {
	CCPass  bool
	CC1Pass bool
	CC2Pass bool

	PairsChecked     int
	DependentSkipped int

	CC1FailEvent1 string
	CC1FailEvent2 string
	CC1FailState  string
	CC1FailNF1    string
	CC1FailNF2    string

	CC2FailEvent   string
	CC2FailState   string
	CC2FailNFState string
	CC2FailNF1     string
	CC2FailNF2     string
}

// containsIdent checks if a string contains an identifier (simple heuristic).
func containsIdent(s, ident string) bool {
	// Simple: check for word boundary match.
	idx := 0
	for {
		pos := strings.Index(s[idx:], ident)
		if pos == -1 {
			return false
		}
		absPos := idx + pos
		before := absPos == 0 || !isIdentChar(s[absPos-1])
		after := absPos+len(ident) >= len(s) || !isIdentChar(s[absPos+len(ident)])
		if before && after {
			return true
		}
		idx = absPos + 1
	}
}

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// Internal helpers.

func (cr *CompiledRegistry) makeEnv(st registry.State) *expr.Env {
	return expr.NewEnv(&cr.Schema, st, cr.EnumLiterals)
}

func (cr *CompiledRegistry) evalValid(st registry.State) (bool, error) {
	env := cr.makeEnv(st)
	for _, invExpr := range cr.InvExprs {
		v, err := expr.EvalBool(invExpr, env)
		if err != nil {
			return false, err
		}
		if !v {
			return false, nil
		}
	}
	return true, nil
}

func (cr *CompiledRegistry) evalGuard(evtIdx int, st registry.State) (bool, error) {
	guard := cr.EvtGuards[evtIdx]
	if guard == nil {
		return true, nil // no guard means always enabled
	}
	return expr.EvalBool(guard, cr.makeEnv(st))
}

func (cr *CompiledRegistry) applyEvent(evtIdx int, st registry.State) (registry.State, error) {
	return cr.applyAssignments(cr.EvtExprs[evtIdx], st)
}

func (cr *CompiledRegistry) applyRepair(repIdx int, st registry.State) (registry.State, error) {
	return cr.applyAssignments(cr.RepExprs[repIdx], st)
}

// applyAssignments applies a set of simultaneous assignments.
// All RHS expressions are evaluated in the pre-state.
func (cr *CompiledRegistry) applyAssignments(assignments map[int]*expr.Node, st registry.State) (registry.State, error) {
	env := cr.makeEnv(st)
	post := make(registry.State, len(st))
	copy(post, st)

	for varIdx, exprNode := range assignments {
		val, err := expr.Eval(exprNode, env) // evaluate in pre-state
		if err != nil {
			return nil, err
		}

		v := cr.Schema.Vars[varIdx]
		switch v.Type {
		case registry.TypeBool:
			if !val.IsBool {
				return nil, fmt.Errorf("assignment to bool %q requires bool value", v.Name)
			}
			if val.Bool {
				post[varIdx] = 1
			} else {
				post[varIdx] = 0
			}
		case registry.TypeEnum:
			if !val.IsInt {
				return nil, fmt.Errorf("assignment to enum %q requires enum value", v.Name)
			}
			if val.Int < 0 || val.Int >= v.Size {
				return nil, fmt.Errorf("assignment to enum %q: value %d out of range [0, %d)", v.Name, val.Int, v.Size)
			}
			post[varIdx] = val.Int
		case registry.TypeInt:
			if !val.IsInt {
				return nil, fmt.Errorf("assignment to int %q requires int value", v.Name)
			}
			if val.Int < v.Min || val.Int > v.Max {
				return nil, fmt.Errorf(
					"SPEC ERROR: assignment to %q computed value %d, allowed range [%d, %d] in state %s",
					v.Name, val.Int, v.Min, v.Max, cr.fmtState(st))
			}
			post[varIdx] = val.Int
		}
	}
	return post, nil
}

// computeNF computes the normal form by iterating compensation.
func (cr *CompiledRegistry) computeNF(sid registry.StateID) (registry.StateID, error) {
	current := sid
	for iter := 0; iter < MaxRepairIter; iter++ {
		if cr.Valid[current] {
			return current, nil
		}
		st := cr.Schema.Decode(current)
		env := cr.makeEnv(st)

		// Apply first violated invariant's repair (in declared order).
		repaired := false
		for ri, invExpr := range cr.InvExprs {
			v, err := expr.EvalBool(invExpr, env)
			if err != nil {
				return -1, err
			}
			if !v {
				// This invariant is violated; apply its repair.
				if ri >= len(cr.RepExprs) {
					return -1, fmt.Errorf("no repair defined for invariant %q", cr.Reg.Invariants[ri].Name)
				}
				newSt, err := cr.applyRepair(ri, st)
				if err != nil {
					return -1, err
				}
				current = cr.Schema.Encode(newSt)
				repaired = true
				break
			}
		}
		if !repaired {
			// All invariants pass but Valid[] says false? Shouldn't happen.
			return current, nil
		}
	}
	st := cr.Schema.Decode(sid)
	return -1, fmt.Errorf("compensation did not terminate within %d steps from state %s",
		MaxRepairIter, cr.fmtState(st))
}

// repairDepth counts how many repair steps from sid to NF.
func (cr *CompiledRegistry) repairDepth(sid registry.StateID) (int, error) {
	current := sid
	for depth := 0; depth < MaxRepairIter; depth++ {
		if cr.Valid[current] {
			return depth, nil
		}
		st := cr.Schema.Decode(current)
		env := cr.makeEnv(st)
		repaired := false
		for ri, invExpr := range cr.InvExprs {
			v, err := expr.EvalBool(invExpr, env)
			if err != nil {
				return 0, err
			}
			if !v {
				newSt, err := cr.applyRepair(ri, st)
				if err != nil {
					return 0, err
				}
				current = cr.Schema.Encode(newSt)
				repaired = true
				break
			}
		}
		if !repaired {
			return depth, nil
		}
	}
	return MaxRepairIter, fmt.Errorf("repair did not terminate")
}

func (cr *CompiledRegistry) fmtState(st registry.State) string {
	parts := make([]string, len(st))
	for i, v := range st {
		vd := cr.Schema.Vars[i]
		switch vd.Type {
		case registry.TypeBool:
			if v == 1 {
				parts[i] = vd.Name + "=true"
			} else {
				parts[i] = vd.Name + "=false"
			}
		case registry.TypeEnum:
			if v >= 0 && v < len(vd.Values) {
				parts[i] = vd.Name + "=" + vd.Values[v]
			} else {
				parts[i] = fmt.Sprintf("%s=?%d", vd.Name, v)
			}
		case registry.TypeInt:
			parts[i] = fmt.Sprintf("%s=%d", vd.Name, v)
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// Stats returns summary statistics.
func (cr *CompiledRegistry) Stats() (validCount, invalidCount int) {
	for sid := 0; sid < cr.Schema.TotalLen; sid++ {
		if cr.Valid[sid] {
			validCount++
		} else {
			invalidCount++
		}
	}
	return
}
