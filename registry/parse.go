package registry

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Raw YAML structures for unmarshaling.

type rawFile struct {
	Registry rawRegistry `yaml:"registry"`
}

type rawRegistry struct {
	Name         string                       `yaml:"name"`
	States       map[string]rawVar            `yaml:"states"`
	Initial      map[string]interface{}        `yaml:"initial"`
	Invariants   map[string]rawInvariant      `yaml:"invariants"`
	Compensation []rawRepair                  `yaml:"compensation"`
	Events       map[string]rawEvent          `yaml:"events"`
}

type rawVar struct {
	Type   string   `yaml:"type"`
	Values []string `yaml:"values"`
	Range  []int    `yaml:"range"`
}

type rawInvariant struct {
	Expr string `yaml:"expr"`
}

type rawRepair struct {
	Invariant string            `yaml:"invariant"`
	Repair    map[string]interface{} `yaml:"repair"`
}

type rawEvent struct {
	Guard  string                 `yaml:"guard"`
	Effect map[string]interface{} `yaml:"effect"`
}

// LoadFile parses a registry YAML file.
func LoadFile(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse parses registry YAML bytes.
func Parse(data []byte) (*Registry, error) {
	var raw rawFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}
	r := &raw.Registry
	if r.Name == "" {
		return nil, fmt.Errorf("registry must have a name")
	}

	reg := &Registry{
		Name:    r.Name,
		Initial: r.Initial,
	}

	// Parse state variables (deterministic order via yaml node ordering).
	// We need stable ordering so re-parse to get key order.
	var ordered struct {
		Registry struct {
			States yaml.Node `yaml:"states"`
		} `yaml:"registry"`
	}
	if err := yaml.Unmarshal(data, &ordered); err != nil {
		return nil, err
	}

	statesNode := &ordered.Registry.States
	if statesNode.Kind == yaml.MappingNode {
		for i := 0; i < len(statesNode.Content)-1; i += 2 {
			keyNode := statesNode.Content[i]
			name := keyNode.Value
			rv, ok := r.States[name]
			if !ok {
				return nil, fmt.Errorf("state var %q not found", name)
			}
			vd, err := parseVarDef(name, rv)
			if err != nil {
				return nil, err
			}
			reg.Vars = append(reg.Vars, vd)
		}
	}

	// Parse invariants (preserve order).
	var invOrdered struct {
		Registry struct {
			Invariants yaml.Node `yaml:"invariants"`
		} `yaml:"registry"`
	}
	if err := yaml.Unmarshal(data, &invOrdered); err != nil {
		return nil, err
	}
	invNode := &invOrdered.Registry.Invariants
	if invNode.Kind == yaml.MappingNode {
		for i := 0; i < len(invNode.Content)-1; i += 2 {
			name := invNode.Content[i].Value
			ri, ok := r.Invariants[name]
			if !ok {
				return nil, fmt.Errorf("invariant %q not found", name)
			}
			reg.Invariants = append(reg.Invariants, Invariant{Name: name, Expr: ri.Expr})
		}
	}

	// Parse compensation (already ordered as list).
	for _, rc := range r.Compensation {
		assignments := make(map[string]string)
		for k, v := range rc.Repair {
			assignments[k] = fmt.Sprintf("%v", v)
		}
		reg.Compensation = append(reg.Compensation, Repair{
			Invariant:   rc.Invariant,
			Assignments: assignments,
		})
	}

	// Parse events (preserve order).
	var evtOrdered struct {
		Registry struct {
			Events yaml.Node `yaml:"events"`
		} `yaml:"registry"`
	}
	if err := yaml.Unmarshal(data, &evtOrdered); err != nil {
		return nil, err
	}
	evtNode := &evtOrdered.Registry.Events
	if evtNode.Kind == yaml.MappingNode {
		for i := 0; i < len(evtNode.Content)-1; i += 2 {
			name := evtNode.Content[i].Value
			re, ok := r.Events[name]
			if !ok {
				return nil, fmt.Errorf("event %q not found", name)
			}
			assignments := make(map[string]string)
			for k, v := range re.Effect {
				assignments[k] = fmt.Sprintf("%v", v)
			}
			reg.Events = append(reg.Events, Event{
				Name:        name,
				Guard:       re.Guard,
				Assignments: assignments,
			})
		}
	}

	return reg, nil
}

func parseVarDef(name string, rv rawVar) (VarDef, error) {
	vd := VarDef{Name: name}
	switch rv.Type {
	case "bool":
		vd.Type = TypeBool
		vd.Size = 2
	case "enum":
		vd.Type = TypeEnum
		vd.Values = rv.Values
		vd.Size = len(rv.Values)
		if vd.Size == 0 {
			return vd, fmt.Errorf("enum %q has no values", name)
		}
	case "int":
		vd.Type = TypeInt
		if len(rv.Range) != 2 {
			return vd, fmt.Errorf("int %q needs range: [min, max]", name)
		}
		vd.Min = rv.Range[0]
		vd.Max = rv.Range[1]
		vd.Size = vd.Max - vd.Min + 1
		if vd.Size <= 0 {
			return vd, fmt.Errorf("int %q has empty range [%d, %d]", name, vd.Min, vd.Max)
		}
	default:
		return vd, fmt.Errorf("unknown type %q for %q", rv.Type, name)
	}
	return vd, nil
}
