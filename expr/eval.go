package expr

import (
	"fmt"

	"github.com/blackwell-systems/nccheck/registry"
)

// Value is a tagged union for evaluation results.
type Value struct {
	IsInt  bool
	IsBool bool
	Int    int
	Bool   bool
}

// Env maps variable names to values, with schema for type info.
type Env struct {
	Schema *registry.Schema
	State  registry.State
	// enumLookup maps enum literal string -> (varIndex, valueIndex) for disambiguation.
	// Populated once per schema.
	EnumLiterals map[string]int // enum literal -> encoded int value
	EnumVarMap   map[string]int // enum literal -> which var index it belongs to (for type checking)
}

// NewEnv creates an evaluation environment from schema + state.
func NewEnv(schema *registry.Schema, state registry.State, enumLiterals map[string]int) *Env {
	return &Env{
		Schema:       schema,
		State:        state,
		EnumLiterals: enumLiterals,
	}
}

// BuildEnumLiterals precomputes a lookup table of enum literal -> encoded value.
// Returns error if any enum literal conflicts with a variable name or another enum.
func BuildEnumLiterals(schema *registry.Schema) (map[string]int, error) {
	varNames := make(map[string]bool)
	for _, v := range schema.Vars {
		varNames[v.Name] = true
	}

	literals := make(map[string]int)
	for _, v := range schema.Vars {
		if v.Type != registry.TypeEnum {
			continue
		}
		for idx, lit := range v.Values {
			if varNames[lit] {
				return nil, fmt.Errorf("enum literal %q conflicts with variable name", lit)
			}
			if _, exists := literals[lit]; exists {
				return nil, fmt.Errorf("enum literal %q appears in multiple enums", lit)
			}
			literals[lit] = idx
		}
	}
	return literals, nil
}

// Eval evaluates an AST node in the given environment.
func Eval(node *Node, env *Env) (Value, error) {
	switch node.Type {
	case NodeLitInt:
		return Value{IsInt: true, Int: node.IntVal}, nil

	case NodeLitBool:
		return Value{IsBool: true, Bool: node.BoolVal}, nil

	case NodeVar:
		// Check if it's a state variable.
		idx := env.Schema.VarIndex(node.Name)
		if idx >= 0 {
			v := env.Schema.Vars[idx]
			switch v.Type {
			case registry.TypeBool:
				return Value{IsBool: true, Bool: env.State[idx] == 1}, nil
			case registry.TypeEnum:
				return Value{IsInt: true, Int: env.State[idx]}, nil
			case registry.TypeInt:
				return Value{IsInt: true, Int: env.State[idx]}, nil
			}
		}
		// Check if it's an enum literal.
		if val, ok := env.EnumLiterals[node.Name]; ok {
			return Value{IsInt: true, Int: val}, nil
		}
		return Value{}, fmt.Errorf("undefined identifier %q", node.Name)

	case NodeNot:
		v, err := Eval(node.Children[0], env)
		if err != nil {
			return Value{}, err
		}
		if !v.IsBool {
			return Value{}, fmt.Errorf("'not' requires bool operand")
		}
		return Value{IsBool: true, Bool: !v.Bool}, nil

	case NodeAnd:
		left, err := Eval(node.Children[0], env)
		if err != nil {
			return Value{}, err
		}
		right, err := Eval(node.Children[1], env)
		if err != nil {
			return Value{}, err
		}
		if !left.IsBool || !right.IsBool {
			return Value{}, fmt.Errorf("'and' requires bool operands")
		}
		return Value{IsBool: true, Bool: left.Bool && right.Bool}, nil

	case NodeOr:
		left, err := Eval(node.Children[0], env)
		if err != nil {
			return Value{}, err
		}
		right, err := Eval(node.Children[1], env)
		if err != nil {
			return Value{}, err
		}
		if !left.IsBool || !right.IsBool {
			return Value{}, fmt.Errorf("'or' requires bool operands")
		}
		return Value{IsBool: true, Bool: left.Bool || right.Bool}, nil

	case NodeEq, NodeNeq:
		left, err := Eval(node.Children[0], env)
		if err != nil {
			return Value{}, err
		}
		right, err := Eval(node.Children[1], env)
		if err != nil {
			return Value{}, err
		}
		eq := false
		if left.IsBool && right.IsBool {
			eq = left.Bool == right.Bool
		} else if left.IsInt && right.IsInt {
			eq = left.Int == right.Int
		} else {
			return Value{}, fmt.Errorf("type mismatch in equality comparison")
		}
		if node.Type == NodeNeq {
			eq = !eq
		}
		return Value{IsBool: true, Bool: eq}, nil

	case NodeLt, NodeLe, NodeGt, NodeGe:
		left, err := Eval(node.Children[0], env)
		if err != nil {
			return Value{}, err
		}
		right, err := Eval(node.Children[1], env)
		if err != nil {
			return Value{}, err
		}
		if !left.IsInt || !right.IsInt {
			return Value{}, fmt.Errorf("comparison requires int operands")
		}
		var result bool
		switch node.Type {
		case NodeLt:
			result = left.Int < right.Int
		case NodeLe:
			result = left.Int <= right.Int
		case NodeGt:
			result = left.Int > right.Int
		case NodeGe:
			result = left.Int >= right.Int
		}
		return Value{IsBool: true, Bool: result}, nil

	case NodeAdd, NodeSub, NodeMul, NodeDiv, NodeMod:
		left, err := Eval(node.Children[0], env)
		if err != nil {
			return Value{}, err
		}
		right, err := Eval(node.Children[1], env)
		if err != nil {
			return Value{}, err
		}
		if !left.IsInt || !right.IsInt {
			return Value{}, fmt.Errorf("arithmetic requires int operands")
		}
		var result int
		switch node.Type {
		case NodeAdd:
			result = left.Int + right.Int
		case NodeSub:
			result = left.Int - right.Int
		case NodeMul:
			result = left.Int * right.Int
		case NodeDiv:
			if right.Int == 0 {
				return Value{}, fmt.Errorf("division by zero")
			}
			result = left.Int / right.Int
		case NodeMod:
			if right.Int == 0 {
				return Value{}, fmt.Errorf("modulo by zero")
			}
			result = left.Int % right.Int
		}
		return Value{IsInt: true, Int: result}, nil

	case NodeIf:
		cond, err := Eval(node.Children[0], env)
		if err != nil {
			return Value{}, err
		}
		if !cond.IsBool {
			return Value{}, fmt.Errorf("if condition must be bool")
		}
		if cond.Bool {
			return Eval(node.Children[1], env)
		}
		return Eval(node.Children[2], env)

	case NodeCall:
		switch node.Name {
		case "min":
			a, err := Eval(node.Children[0], env)
			if err != nil {
				return Value{}, err
			}
			b, err := Eval(node.Children[1], env)
			if err != nil {
				return Value{}, err
			}
			if !a.IsInt || !b.IsInt {
				return Value{}, fmt.Errorf("min requires int arguments")
			}
			if a.Int < b.Int {
				return a, nil
			}
			return b, nil
		case "max":
			a, err := Eval(node.Children[0], env)
			if err != nil {
				return Value{}, err
			}
			b, err := Eval(node.Children[1], env)
			if err != nil {
				return Value{}, err
			}
			if !a.IsInt || !b.IsInt {
				return Value{}, fmt.Errorf("max requires int arguments")
			}
			if a.Int > b.Int {
				return a, nil
			}
			return b, nil
		case "clamp":
			lo, err := Eval(node.Children[0], env)
			if err != nil {
				return Value{}, err
			}
			x, err := Eval(node.Children[1], env)
			if err != nil {
				return Value{}, err
			}
			hi, err := Eval(node.Children[2], env)
			if err != nil {
				return Value{}, err
			}
			if !lo.IsInt || !x.IsInt || !hi.IsInt {
				return Value{}, fmt.Errorf("clamp requires int arguments")
			}
			v := x.Int
			if v < lo.Int {
				v = lo.Int
			}
			if v > hi.Int {
				v = hi.Int
			}
			return Value{IsInt: true, Int: v}, nil
		default:
			return Value{}, fmt.Errorf("unknown function %q", node.Name)
		}

	default:
		return Value{}, fmt.Errorf("unknown node type %d", node.Type)
	}
}

// EvalBool is a convenience for evaluating a boolean expression.
func EvalBool(node *Node, env *Env) (bool, error) {
	v, err := Eval(node, env)
	if err != nil {
		return false, err
	}
	if !v.IsBool {
		return false, fmt.Errorf("expected bool expression, got int")
	}
	return v.Bool, nil
}
