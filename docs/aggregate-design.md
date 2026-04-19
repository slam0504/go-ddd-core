# Aggregate design

The DDD primitives in `domain/` are small. Most of the work is using them
correctly. This page is the rule of thumb plus a worked example.

## Anemic vs rich aggregate

| Symptom                                        | Anemic                       | Rich                                              |
|------------------------------------------------|------------------------------|---------------------------------------------------|
| What lives on the struct                        | Fields + getters + setters   | Fields + behaviour methods                        |
| Where invariants live                           | Scattered across services    | On the aggregate, enforced before any mutation    |
| Test that proves a rule                         | Reads many service files     | One method test on the aggregate                  |
| Adding a new rule                              | Hunt the call sites          | Edit the relevant method                          |
| Public API surface                             | One getter + one setter per field | A handful of intent-named methods (`Activate`, `Cancel`, `Apply`)   |

When in doubt: if removing every getter and setter would break business
logic in other packages, the logic belongs on the aggregate, not in those
packages.

## When to use `BaseAggregate[ID]`

Use [`domain.BaseAggregate[ID]`](../domain/aggregate.go) for:

- **Event recording.** `Record(event)` collects events that should be
  dispatched after persistence; `DomainEvents()` returns a defensive copy;
  `ClearEvents()` is called once events have been safely staged (Outbox or
  immediate publish).
- **Optimistic concurrency.** `Version()`, `SetVersion(v)`,
  `IncrementVersion()` let repositories detect lost updates against a
  numeric version column.
- **Identity.** `BaseEntity[ID].Equal(other)` compares by `ID`, not by
  fields, which is what aggregate identity means.

You do *not* need `BaseAggregate` for value objects (no identity, no
events) or for read-model DTOs (no behaviour, no events).

## Enforcing invariants

Every method that mutates state must:

1. **Read** the receiver's current state.
2. **Validate** the precondition. On failure, return
   [`domain.NewRuleViolation(code, message)`](../domain/errors.go) — never
   panic, never log-and-continue.
3. **Mutate** fields atomically (no `time.Sleep`, no I/O).
4. **Record** a domain event capturing the fact (past tense:
   `OrderCancelled`, not `CancelOrder`).
5. **Return** `nil` on success, `error` on rule violation.

```go
func (o *Order) Cancel(reason string) error {
    if o.status == OrderStatusFulfilled {
        return domain.NewRuleViolation("ORDER_ALREADY_FULFILLED",
            "fulfilled orders cannot be cancelled")
    }
    if reason == "" {
        return domain.NewRuleViolation("CANCEL_REASON_REQUIRED",
            "cancellation reason is mandatory")
    }
    o.status = OrderStatusCancelled
    o.cancelReason = reason
    o.Record(NewOrderCancelled(o.ID(), reason))
    return nil
}
```

Two failure cases, two distinct codes — callers can match on the code
without parsing strings.

## Inner entities and value objects

Splitting an aggregate is a structural decision, not a stylistic one:

- **Value object** when identity does not matter (Money, EmailAddress,
  DateRange). It implements `domain.ValueObject` and is immutable. Use
  `domain.EqualValues` or `domain.DeepEqualValues` inside `Equal`.
- **Inner entity** when identity within the aggregate matters but the
  entity has no independent lifecycle (an `OrderLine` inside an `Order`).
  The inner entity's mutations are *only* reachable through the aggregate
  root.
- **Separate aggregate** when the entity has its own lifecycle and
  consistency boundary. Reference it by ID, not by pointer.

If you find yourself reaching across aggregate boundaries to mutate state,
the boundary is in the wrong place — usually the two should be one
aggregate, or the cross-aggregate change should go through a domain event.

## When to record a domain event

Record an event when a fact about the world has changed that other parts
of the system might care about. Events are nouns in past tense.

- ✅ `OrderPlaced`, `PaymentReceived`, `ScheduleActivated`
- ❌ `PlaceOrder`, `UpdateStatus`, `SaveSchedule` (those are commands or
  CRUD verbs, not facts)

Record at the point of mutation, not in the application layer. Domain
events are part of the aggregate's contract; if a method causes the
mutation, the same method should record the event.

After persistence the application layer dispatches the events (via the
Outbox pattern for at-least-once delivery, see
[`docs/anti-patterns.md`](anti-patterns.md#6-at-least-once-delivery-only-on-the-producer-side)).
Once dispatched, call `ClearEvents()`.

## Worked example: BankAccount

The full BankAccount aggregate lives in
[`examples/minimal/domain/account/`](../examples/minimal/domain/account)
as compilable, tested code:

- [`account.go`](../examples/minimal/domain/account/account.go) — aggregate
  with `New`, `Withdraw`, `Deposit`, `Freeze`, `Close`,
  `SetOverdraftLimit`. Every method enforces multiple invariants and
  records a domain event on success.
- [`events.go`](../examples/minimal/domain/account/events.go) — six event
  types, all named in past tense (`AccountOpened`, `MoneyWithdrawn`, …).
- [`account_test.go`](../examples/minimal/domain/account/account_test.go) —
  21 tests covering the happy path *and* every rule violation, with a
  helper that asserts on `*domain.RuleViolation.Code` so tests stay
  meaningful when messages are reworded.

The shape of a single method (the most rule-dense one) reads:

```go
func (a *Account) Withdraw(amount int64, eventID string) error {
    switch a.status {
    case StatusFrozen:
        return domain.NewRuleViolation("ACCOUNT_FROZEN",
            "withdrawals are blocked while the account is frozen")
    case StatusClosed:
        return domain.NewRuleViolation("ACCOUNT_CLOSED",
            "withdrawals are not allowed on closed accounts")
    }
    if amount <= 0 {
        return domain.NewRuleViolation("AMOUNT_NOT_POSITIVE",
            "withdrawal amount must be positive")
    }
    if a.balance-amount < -a.overdraft {
        return domain.NewRuleViolation("OVERDRAFT_EXCEEDED",
            "withdrawal would exceed allowed overdraft")
    }
    a.balance -= amount
    a.IncrementVersion()
    a.Record(NewMoneyWithdrawn(eventID, string(a.ID()), a.Version(), amount, a.balance))
    return nil
}
```

Four distinct rule codes, four call sites that can match on them, and one
event recorded only after every check has passed. Compare with an anemic
equivalent (`SetBalance(newBalance int64)`) and the difference is the
entire point of this page.

What this aggregate does *not* expose:

- ❌ `SetStatus(s Status)` — there is no situation where an external
  caller should set the status arbitrarily; the rules live in `Freeze`,
  `Close`, etc.
- ❌ `SetBalance(b int64)` — balance changes are events of `Withdraw`,
  `Deposit`, `ApplyInterest`, never raw assignment.
- ❌ Persistence concerns (no GORM tags, no DB column metadata, no
  serialisation directives) — those belong in the adapter's model type.

The example is run by `go test ./examples/minimal/domain/account/...` and
fails immediately if a future change to `domain.BaseAggregate` breaks the
shape — the docs and the code are kept honest by the same test command.

## Common smells (and what they usually mean)

| Smell                                                    | Likely cause                                                                  |
|----------------------------------------------------------|-------------------------------------------------------------------------------|
| Aggregate has 10+ setter methods                         | Anemic — move logic from services onto the aggregate                          |
| Application service code reads `if x.GetStatus() == ...` | The decision belongs to a method on `x`, not on the service                   |
| Aggregate has no events recorded after a mutation        | Either the mutation is private state-tracking, or you are missing an event    |
| Two aggregates mutate each other in one call             | Boundary is wrong — either merge them, or make the change asynchronous via an event   |
| Aggregate field types are `string` everywhere            | Likely missing value objects (`OrderID`, `EmailAddress`, `Money`)             |
| Tests instantiate aggregates with `&Aggregate{...}` literals | Constructor is missing — invariants at creation time are not enforced         |
