# nccheck DSL Specification
# Types, expressions, and evaluation rules.

## Type System

Three types. All finite. No subtyping.

    bool     values: true, false
    enum(V)  values: members of V (e.g., enum([pending, paid, shipped]))
    int(a,b) values: integers in [a, b] inclusive

Type checking is static (at spec parse time, before enumeration).

## Expression Grammar (Pratt parser, precedence low→high)

    expr     = ternary
    ternary  = logic ( "if" logic "then" expr "else" expr )?
    logic    = compare ( ("and" | "or") compare )*
    compare  = arith ( ("==" | "!=" | "<" | "<=" | ">" | ">=") arith )?
    arith    = unary ( ("+" | "-") unary )*
    factor   = unary ( ("*" | "/" | "%") unary )*
    unary    = "not" unary | atom
    atom     = "(" expr ")"
             | "true" | "false"
             | INTEGER
             | IDENTIFIER              -- variable reference or enum literal
             | "min" "(" expr "," expr ")"
             | "max" "(" expr "," expr ")"
             | "clamp" "(" expr "," expr "," expr ")"

## Built-in Functions (pure, total)

    min(a, b)        → int: smaller of a, b
    max(a, b)        → int: larger of a, b
    clamp(lo, x, hi) → int: max(lo, min(x, hi))

No other functions. No user-defined functions.

## Type Rules

    not e              : bool → bool
    e1 and e2          : bool × bool → bool
    e1 or e2           : bool × bool → bool
    e1 == e2           : T × T → bool  (T must match: bool==bool, enum==enum, int==int)
    e1 != e2           : T × T → bool
    e1 < e2            : int × int → bool  (also <=, >, >=)
    e1 + e2            : int × int → int   (also -, *, /, %)
    if c then a else b : bool × T × T → T  (branches must match type)
    min(a, b)          : int × int → int
    max(a, b)          : int × int → int
    clamp(lo, x, hi)   : int × int × int → int

## Evaluation Rules

- All expressions are pure and total.
- Division by zero: SPEC ERROR at parse/validation time if divisor can be zero
  (conservative: reject if divisor is not a nonzero literal).
- Integer overflow: SPEC ERROR if result falls outside the variable's declared range
  during *assignment* (not during intermediate computation).
  The error includes: state, event/repair, assignment, computed value, allowed range.
- Enum equality: only == and != are permitted. No ordering on enums.
- Bool: no arithmetic. No ordering. Only == != and or not.

## Assignment Rules (effects and repairs)

An assignment is: variable_name: expression

The expression is evaluated in the *current* state (before any assignments
in this block take effect). All assignments in a block are simultaneous —
they read the pre-state and write the post-state. This eliminates
order-dependence within a single event or repair step.

## State Enumeration

Total state space = cartesian product of all variable domains.
  bool: 2 values
  enum(V): |V| values
  int(a,b): b - a + 1 values

States are bitpacked as indices for table construction.
Tool reports total state space size and refuses if > configurable limit
(default: 1,000,000 states).

## Identifier Resolution

Identifiers resolve in this order:
  1. State variable names
  2. Enum literal values (across all declared enums)

Ambiguity (a state variable named same as an enum value) is a SPEC ERROR.
