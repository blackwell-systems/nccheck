package registry

// VarType represents the type of a state variable.
type VarType int

const (
	TypeBool VarType = iota
	TypeEnum
	TypeInt
)

// VarDef defines a single state variable.
type VarDef struct {
	Name   string
	Type   VarType
	Values []string // for enum
	Min    int      // for int range
	Max    int      // for int range
	Size   int      // number of possible values
}

// Invariant is a named boolean predicate over state.
type Invariant struct {
	Name string
	Expr string
}

// Repair is a compensation step targeting one invariant.
type Repair struct {
	Invariant   string
	Assignments map[string]string // var -> expression string
}

// Event is a named transition with optional guard and effects.
type Event struct {
	Name        string
	Guard       string            // optional boolean expression
	Assignments map[string]string // var -> expression string
}

// Registry is the complete spec for a single registry.
type Registry struct {
	Name         string
	Vars         []VarDef
	Initial      map[string]interface{}
	Invariants   []Invariant
	Compensation []Repair
	Events       []Event
}

// State is a concrete valuation: variable index -> value (int-encoded).
// For bool: 0=false, 1=true
// For enum: index into VarDef.Values
// For int:  actual value (within range)
type State []int

// StateID is a bitpacked integer encoding of a State.
type StateID int

// Schema holds precomputed metadata for state enumeration.
type Schema struct {
	Vars     []VarDef
	Strides  []int // multiplier for each var to compute StateID
	TotalLen int   // total number of states
}

// NewSchema precomputes the schema from variable definitions.
func NewSchema(vars []VarDef) Schema {
	s := Schema{Vars: vars, Strides: make([]int, len(vars))}
	s.TotalLen = 1
	for i := len(vars) - 1; i >= 0; i-- {
		s.Strides[i] = s.TotalLen
		s.TotalLen *= vars[i].Size
	}
	return s
}

// Encode packs a state into a StateID.
func (s *Schema) Encode(st State) StateID {
	id := 0
	for i, v := range st {
		val := v
		if s.Vars[i].Type == TypeInt {
			val = v - s.Vars[i].Min // normalize to 0-based
		}
		id += val * s.Strides[i]
	}
	return StateID(id)
}

// Decode unpacks a StateID into a state.
func (s *Schema) Decode(id StateID) State {
	st := make(State, len(s.Vars))
	rem := int(id)
	for i := range s.Vars {
		st[i] = rem / s.Strides[i]
		rem = rem % s.Strides[i]
		if s.Vars[i].Type == TypeInt {
			st[i] += s.Vars[i].Min // denormalize from 0-based
		}
	}
	return st
}

// VarIndex returns the index of a variable by name, or -1.
func (s *Schema) VarIndex(name string) int {
	for i, v := range s.Vars {
		if v.Name == name {
			return i
		}
	}
	return -1
}

// EnumIndex returns the int encoding of an enum literal within a variable.
func (s *Schema) EnumIndex(varIdx int, value string) int {
	for i, v := range s.Vars[varIdx].Values {
		if v == value {
			return i
		}
	}
	return -1
}
