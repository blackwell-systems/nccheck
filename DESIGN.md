# nccheck spec format — design decisions to resolve

## DECIDED

1. **Language**: Go
2. **Format**: YAML
3. **Variables**: finite by construction (bool, enum, int[min..max])
4. **Compensation**: deterministic — declared priority order, first match fires
5. **Exhaustive verification**: enumerate all states, precompute NF and Step tables

## NEEDS YOUR INPUT

### A. Guarded events: no-op or excluded?

When an event's guard is false, two options:

  Option 1: Event is a no-op (apply returns same state)
  Option 2: Event is inapplicable (excluded from that state's rewrite rules)

Option 1 is simpler — Step(e, s) is always defined, CC1 checks all pairs
at all states. A no-op trivially commutes with anything, so false guards
automatically pass CC1 for those states.

Option 2 is more precise but requires tracking which events are enabled
at which states, and CC1 only applies to co-enabled pairs.

Recommendation: Option 1 (no-op). Simpler, and no-ops don't hurt — they
just add harmless checks that trivially pass.

### B. Compensation firing: first-match or all-matching?

When multiple invariants are violated:

  Option 1: Fire FIRST matching repair, re-evaluate, repeat
  Option 2: Fire ALL matching repairs (in order), then re-evaluate

Option 1 matches the paper's "single ρ application" model more closely —
each application of ρ fires one repair, and iterated ρ* runs to fixpoint.

Option 2 is more efficient (fewer iterations) but changes the semantics —
ρ becomes "repair everything in one pass."

Recommendation: Option 1 (first-match, iterate). Matches the paper.
The tool verifies termination anyway, so iteration count doesn't matter.

### C. Expression language: how much arithmetic?

Minimal (recommended):
  - comparisons: ==, !=, <, <=, >, >=
  - boolean: and, or, not, implies
  - arithmetic: +, -, min, max (no *, /, %)
  - saturating semantics at bounds (inventory + 1 at max stays at max)

Extended:
  - add *, /, % (still bounded — result clamped to variable range)
  - add ternary: if cond then x else y

How much do real compensation rules need?

### D. Federation: inline or multi-file?

Option 1: Each registry is its own .yaml file, federation is a separate
file that references them:

  federation.yaml → references manufacturer.yaml, supplier.yaml

Option 2: Everything in one file with multiple registry blocks.

Recommendation: Option 1 (multi-file). Matches organizational boundaries —
each org owns its own spec. Federation spec is the contract layer.

### E. State space limit

The tool should refuse specs that enumerate too many states.
What's the default ceiling?

  2^16 = 65K states    — instant
  2^20 = 1M states     — seconds  
  2^24 = 16M states    — minutes
  2^28 = 256M states   — probably too slow for "feels instant"

Recommendation: default 2^20, configurable via --max-states flag.

### F. Output format

Plain text (human-readable) by default.
Also support --json for toolchain integration?

### G. Project name

  nccheck     — terse, matches the paper
  ncverify    — clearer about what it does
  regcheck    — generic
  confluent   — memorable but vague

Leaning nccheck. Matches the drainability ecosystem naming
(libdrainprof → nccheck).