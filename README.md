# nccheck — Normalization Confluence Verifier

**Reference implementation** for verifying coordination-free convergence in registry-governed stream systems.

Given a finite-state system specification with invariants and compensation, nccheck exhaustively verifies whether all processors consuming the same events will converge to the same valid state regardless of application order.

**Companion tool to:** [*Normalization Confluence in Federated Registry Networks*](https://doi.org/10.5281/zenodo.18677400) (Blackwell, 2026)

## Quick Start

```bash
go build -o nccheck .
./nccheck examples/disjoint.yaml
```

Exit code 0 if convergence is guaranteed, 1 otherwise.

## What It Checks

The tool verifies two structural conditions from the paper:

1. **WFC (Well-Founded Compensation)**: Compensation terminates from every state and reaches a valid fixpoint
2. **CC (Compensation Commutativity)**: Different event orderings converge to the same normal form

Specifically:
- **CC1**: For independent event pairs, applying in either order produces the same normal form
- **CC2**: Applying an event to an invalid state then compensating produces the same result as compensating first then applying the event

The tool exhaustively enumerates the finite state space, precomputes normal forms and step tables, then checks CC via table lookups. This is **sound and complete** for the declared model.

## Example: PASS

```bash
$ ./nccheck examples/disjoint.yaml
```

```
nccheck — Normalization Confluence Verifier
════════════════════════════════════════════

Registry:    disjoint_tracks
Source:      examples/disjoint.yaml

State Space
  Variables: review:enum(4) × publish:enum(4)
  Total:     16 states
  Valid:     9
  Invalid:   7

Events:      8  [submit, approve, reject, reset_review, ...]
Invariants:  2  [no_draft_after_submit, no_unpublished_after_stage]

WFC (Well-Founded Compensation)
  Result:    PASS
  Max depth: 2

CC (Compensation Commutativity)
  CC1:       PASS  (16 independent pairs checked, 12 dependent skipped)
  CC2:       PASS

════════════════════════════════════════════
Unique Normal Form:  YES
Convergence:         GUARANTEED
Checked in:          1.892ms
```

This system has two independent subsystems (review workflow and publish workflow). Each variable has its own invariant. No cross-variable constraints. Convergence is guaranteed.

## Example: FAIL

```bash
$ ./nccheck examples/permissions.yaml
```

```
nccheck — Normalization Confluence Verifier
════════════════════════════════════════════

Registry:    permissions
Source:      examples/permissions.yaml

State Space
  Variables: can_read:bool × can_write:bool
  Total:     4 states
  Valid:     3
  Invalid:   1

Events:      4  [grant_read, revoke_read, grant_write, revoke_write]
Invariants:  1  [write_needs_read]

WFC (Well-Founded Compensation)
  Result:    PASS
  Max depth: 1

CC (Compensation Commutativity)
  CC1:       FAIL
    Events:  (grant_read, grant_write)
    State:   {can_read=false, can_write=false}
    Order 1: grant_read → grant_write → {can_read=true, can_write=true}
    Order 2: grant_write → grant_read → {can_read=true, can_write=false}
  CC2:       FAIL
    Event:   grant_read
    State:   {can_read=false, can_write=true}
    NF(s):   {can_read=false, can_write=false}
    Step(e,s):     → {can_read=true, can_write=true}
    Step(e,NF(s)): → {can_read=true, can_write=false}

════════════════════════════════════════════
Convergence:         NOT GUARANTEED
  ✗ CC1 failed
  ✗ CC2 failed
Checked in:          996µs
```

This system has a cross-variable invariant: `write_needs_read: "can_write implies can_read"`. When `grant_write` arrives before `grant_read`, compensation revokes write permission. Different event orderings reach different valid states. **Convergence is not guaranteed.**

The tool shows the exact state, event pair, and divergent traces where CC fails.

## Registry Spec Format

Registry specs are YAML files declaring:

```yaml
registry:
  name: my_system

  states:
    my_var:
      type: enum  # or bool, or int with range [min, max]
      values: [state1, state2, state3]

  initial:
    my_var: state1

  invariants:
    my_rule:
      expr: "my_var != state1"  # Boolean expression

  compensation:
    - invariant: my_rule
      repair:
        my_var: state2  # Absolute assignment

  events:
    my_event:
      guard: "my_var == state1"  # Optional precondition
      effect:
        my_var: state3
```

**Supported types:**
- `bool` — true/false (2 states)
- `enum` — named values (N states)
- `int` with `range: [min, max]` — bounded integer (inclusive)

All state spaces must be finite. The tool refuses specs exceeding 2²⁰ ≈ 1M states by default.

See `SPEC_DRAFT.yaml` for the full DSL specification.

## When Compensation Fails to Commute

The tool makes precise what the paper proves theoretically: **CC is hard to satisfy when compensation is destructive (rolls back state) and invariants are cross-variable (one variable constrains another).**

Common failure patterns:
- **Cross-variable invariants**: `A implies B` means modifying A affects B
- **Destructive compensation**: Rolling back to a previous state loses information from events
- **Guard conditions**: Events that check multiple variables create dependencies

The tool shows exactly where and why CC fails, enabling compensation redesign.

## Examples Provided

```
examples/disjoint.yaml          # PASS - independent subsystems
examples/permissions.yaml       # FAIL - cross-variable invariants
examples/order_fulfillment.yaml # FAIL - destructive compensation
examples/workflow.yaml          # Complex state machine
examples/traffic_light.yaml     # Cyclic state transitions
examples/counters.yaml          # Bounded integer operations
examples/two_flags.yaml         # Minimal boolean system
examples/independent.yaml       # Independence declarations
examples/access_control.yaml    # Role-based permissions
```

## Relationship to the Paper

This tool implements the verification procedure described in Section 9 of the paper. It:

1. Enumerates the finite state space Σ (cartesian product of variable domains)
2. Computes ρ*(σ) for all σ ∈ Σ (normal forms via iterated compensation)
3. Verifies WFC by checking that ρ* terminates and reaches valid states
4. Builds the Step table: Step(e, σ) = ρ*(apply(e, σ)) for all events e and states σ
5. Checks CC1 by comparing Step(e₁, Step(e₂, σ)) with Step(e₂, Step(e₁, σ))
6. Checks CC2 by comparing Step(e, σ) with Step(e, ρ*(σ))

The paper proves: **WFC + CC ⟹ unique normal forms** (Theorem 5.1, via Newman's Lemma).

The tool verifies: **does this spec satisfy WFC + CC?**

## What This Tool Is

A **verification tool for finite-state registry-governed systems**. Given a complete formal specification, it provides mathematical proof of convergence or counterexamples showing exactly why convergence fails.

## What This Tool Is Not

- Not a runtime monitor (operates on specs, not running systems)
- Not a testing framework (exhaustive verification, not sampling)
- Not applicable to unbounded state spaces (requires finite domains)
- Not a general distributed systems debugger

This is a **reference implementation** demonstrating that the paper's verification procedure is mechanizable. It proves the theory is concrete, not just abstract.

## License

MIT License - see LICENSE file

## Citation

```bibtex
@techreport{blackwell2026nc,
  author = {Blackwell, Dayna},
  title = {Normalization Confluence in Federated Registry Networks},
  year = {2026},
  publisher = {Zenodo},
  doi = {10.5281/zenodo.18677400},
  url = {https://doi.org/10.5281/zenodo.18677400}
}
```
