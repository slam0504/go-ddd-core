# Anti-patterns this core helps you avoid

`go-ddd-core` is small on purpose. Many of its APIs exist *because* a specific
production anti-pattern hurt — losing test isolation, leaking infrastructure
into the domain, dropping events, or scaling map-typed search until the team
could no longer reason about it.

This document collects those anti-patterns with concrete substitutes from
core. The list is distilled from real production Go services that grew anemic
over time; symptoms are described generically rather than naming any service.

If you are reviewing code and recognise a pattern below, the substitute
section tells you which core API to reach for.

---

## 1. Service Locator inside use cases

**Symptom.** A use case reaches into a global container at runtime to pull
its dependencies:

```go
// ❌ Anti-pattern
func (uc *DispatchScheduleUseCase) Execute(ctx context.Context, in Input) error {
    return foundation.App.MakeEvent().DispatchWithContext(ctx, &ScheduleStarted{...})
    //     ^^^^^^^^^^^^^^^^^^^^^^^^^^^ resolves a global at call time
}
```

**Why it's bad.**
- The use case silently depends on the framework — you can't unit-test it
  without booting the whole container, and the dependency is invisible at
  the call site.
- Stubbing one collaborator (the event bus) for one test means rewriting
  `App.Make*` for that test, which leaks across other tests.
- Dependencies become discoverable only by reading the body of every method.

**Substitute.** Constructor injection. Resolve adapters once at startup via
[`bootstrap.AdapterRegistry`](../bootstrap/registry.go) and inject them into
the use case as plain interface fields:

```go
// ✅ Use this
type DispatchScheduleUseCase struct {
    publisher eventbus.Publisher  // injected interface
}

func NewDispatchScheduleUseCase(p eventbus.Publisher) *DispatchScheduleUseCase {
    return &DispatchScheduleUseCase{publisher: p}
}

func (uc *DispatchScheduleUseCase) Execute(ctx context.Context, in Input) error {
    return uc.publisher.Publish(ctx, "schedule.events", &ScheduleStarted{...})
}
```

`AdapterRegistry.Resolve[T]` performs the lookup once at wiring time, in one
place — not at every call site.

---

## 2. Static facade over infrastructure

**Symptom.** A package-level helper that *looks* like a free function but
internally calls the global container:

```go
// ❌ Anti-pattern
package facades

func EventBus() eventbus.Publisher {
    return foundation.App.MakeEvent()  // service locator in disguise
}

// caller:
facades.EventBus().Publish(ctx, topic, evt)
```

**Why it's bad.** It is the anti-pattern from #1 with a friendlier signature.
Tests still need a global container; dependencies are still invisible at the
call site; the indirection now hides *more* than it explains.

**Substitute.** Inject the [`eventbus.Publisher`](../eventbus/bus.go)
interface directly. There is no shorthand worth this trade-off.

---

## 3. Anemic aggregate

**Symptom.** A "domain entity" that is purely a bag of fields with no
behaviour:

```go
// ❌ Anti-pattern
type Schedule struct { /* 15 fields */ }

func (s *Schedule) GetID() string         { return s.id }
func (s *Schedule) SetID(v string)        { s.id = v }
func (s *Schedule) GetStatus() string     { return s.status }
func (s *Schedule) SetStatus(v string)    { s.status = v }   // arbitrary mutation, no rules
// ... 13 more setter pairs, zero invariants
```

Business rules end up in services or controllers (`if schedule.GetStatus() ==
"pending" { schedule.SetStatus("active") }`), scattered and re-implemented
per call site.

**Why it's bad.**
- Invariants cannot be enforced — any caller can call `SetStatus("invalid")`.
- Behaviour is duplicated across services: every place that "activates a
  schedule" re-derives the rules.
- Changes to a rule force a hunt through call sites; the aggregate cannot
  tell you the truth.

**Substitute.** Put behaviour and invariants on the aggregate itself. Use
[`domain.BaseAggregate[ID]`](../domain/aggregate.go) for event recording and
versioning, and return [`domain.RuleViolation`](../domain/errors.go) when an
invariant fails:

```go
// ✅ Use this
type Schedule struct {
    domain.BaseAggregate[string]
    status   ScheduleStatus
    nextRun  time.Time
}

func (s *Schedule) Activate(at time.Time) error {
    if s.status != ScheduleStatusPending {
        return domain.NewRuleViolation("SCHEDULE_NOT_PENDING",
            "only pending schedules can be activated")
    }
    s.status = ScheduleStatusActive
    s.nextRun = at
    s.Record(NewScheduleActivated(s.ID(), at))
    return nil
}
```

Now there is exactly one place that knows the rule, exactly one place that
mutates the field, and exactly one event that records the fact.

See [`docs/aggregate-design.md`](aggregate-design.md) for a longer worked
example.

---

## 4. Map-typed query and patch

**Symptom.** A repository contract that takes a `map[string]any` for filters
or partial updates:

```go
// ❌ Anti-pattern
type Repository[T any, PK any] interface {
    Search(ctx context.Context, criteria map[string]any) ([]T, error)
    Patch(ctx context.Context, fields map[string]any) (int64, error)
}

// caller:
users, _ := repo.Search(ctx, map[string]any{"age_gte": 18, "status": "active"})
_, _    = repo.Patch(ctx, map[string]any{"id": "u-1", "status": "banned"})
```

**Why it's bad.**
- No compile-time check on field names — typos surface as silent zero
  results or no-op updates.
- No type safety on values — passing `int` where `int64` is expected becomes
  a runtime cast at the adapter.
- Refactoring a column rename does not compile-fail; it produces missed
  filters and partial updates.
- The contract leaks the storage shape into every caller.

**Substitute.** Two pieces from core:

- For queries: build a tree with
  [`application/query/spec.Specification[T]`](../application/query/spec/spec.go).
  Adapter-defined leaves opt into SQL via `spec.SQLTranslatable`.
- For partial updates: use a typed update struct (REST: PATCH input schema;
  GraphQL: input type). The adapter knows which fields are present.

```go
// ✅ Use this
spec := userAgeAtLeast{Min: 18}.And(userStatusIs{Status: "active"})
page, _ := store.Search(ctx, spec, pagination.Limit{Size: 20})

type UpdateUserStatus struct {
    ID     string
    Status UserStatus
}
_ = store.UpdateStatus(ctx, UpdateUserStatus{ID: "u-1", Status: UserStatusBanned})
```

---

## 5. Dual error wrapping

**Symptom.** Two parallel ways to attach a code to an error, used
interchangeably:

```go
// ❌ Anti-pattern
return pirestful.WrapError(err, pierror.NotFound)         // *wrapError holds pierror.Code
return pirestful.InheritError(err, "USER_NOT_FOUND")      // *inheritError holds string
```

Now every adapter has to type-assert against two wrapper types to recover
the code, and the codes themselves live in two namespaces.

**Why it's bad.** Two ways to do the same thing means callers handle both
inconsistently, and the team eventually adds a third "for a special case".

**Substitute.** A single coded error type:
[`pkg/errorsx.Error`](../pkg/errorsx/errorsx.go).

```go
// ✅ Use this
return errorsx.Wrap(errorsx.CodeNotFound, "user not found", err).
    WithDetail("user_id", id)
```

Plus the transport bridges that translate it consistently:
- [`pkg/errorsx/httpx`](../pkg/errorsx/httpx/mapping.go) — JSON body + HTTP status
- [`transport/grpc.CodeFromErrorsx`](../transport/grpc/errorx.go) — gRPC status

There is exactly one error type, one set of codes, one set of mappers.

---

## 6. At-least-once delivery, only on the producer side

**Symptom.** Domain events go through a `Publisher.Publish()` immediately
after the aggregate save, with no Outbox in between, and consumers have no
Inbox to deduplicate retried messages.

```go
// ❌ Anti-pattern
if err := repo.Save(ctx, order); err != nil { return err }
if err := publisher.Publish(ctx, "orders", order.DomainEvents()...); err != nil {
    return err  // aggregate saved, event lost — broken atomicity
}
```

**Why it's bad.**
- Save-then-publish is not atomic. A crash between the two leaves the
  aggregate persisted and the event lost.
- Even if publish succeeds, the broker may redeliver. Without an Inbox,
  consumers process the same event multiple times and corrupt downstream
  state.

**Substitute.** Both ends of the at-least-once pattern:

- Producer side: [`eventbus.Outbox`](../eventbus/outbox.go) stages events in
  the same transaction as the aggregate save; an
  [`eventbus.Relay`](../eventbus/outbox.go) drains them to the broker.
- Consumer side: [`eventbus/inbox.Memory`](../eventbus/inbox/memory.go) (or
  an SQL adapter from `go-ddd-adapters`) deduplicates redelivered messages
  via `Seen` / `Record` around handler invocation.

```go
// ✅ Use this (producer)
err := txm.Within(ctx, func(ctx context.Context) error {
    if err := repo.Save(ctx, order); err != nil { return err }
    return outbox.Stage(ctx, "orders", order.DomainEvents()...)
})

// ✅ Use this (consumer)
seen, _ := inbox.Seen(ctx, env.Event.EventID())
if seen { env.Ack(); return nil }
if err := handler(ctx, env.Event); err != nil { env.Nack(); return err }
_ = inbox.Record(ctx, env.Event.EventID())
env.Ack()
```

Implementing only one half is worse than implementing neither: it gives a
false sense of guarantee.

---

## 7. ORM tags inside domain entities

**Symptom.** The aggregate struct carries persistence metadata:

```go
// ❌ Anti-pattern
type Order struct {
    domain.BaseAggregate[string] `gorm:"-"`
    ID         string `gorm:"column:id;primaryKey;type:BINARY(16)"`
    CustomerID string `gorm:"column:customer_id;index"`
    TotalCents int64  `gorm:"column:total_cents"`
    Status     string `gorm:"column:status;type:varchar(32)"`
}
```

**Why it's bad.**
- Domain layer now depends transitively on a specific ORM. Swapping persistence
  technology means rewriting the aggregate.
- Storage decisions (column names, types, indexes) leak into where business
  rules live, which inverts what the domain layer is for.
- The aggregate cannot be reused outside an ORM context (e.g. event
  sourcing, unit tests with in-memory stores) without ignoring half its
  metadata.

**Substitute.** Keep the domain struct pure. Put ORM tags on a separate
*model* type inside the adapter, and translate at the repository boundary:

```go
// ✅ Use this — domain layer (ORM-free)
type Order struct {
    domain.BaseAggregate[string]
    customerID string
    totalCents int64
    status     OrderStatus
}

// ✅ Use this — adapter layer (gorm-specific)
package gormorder

type model struct {
    ID         string `gorm:"column:id;primaryKey;type:BINARY(16)"`
    CustomerID string `gorm:"column:customer_id;index"`
    TotalCents int64  `gorm:"column:total_cents"`
    Status     string `gorm:"column:status;type:varchar(32)"`
}

func toDomain(m model) order.Order { ... }
func fromDomain(o order.Order) model { ... }
```

The adapter owns the storage shape; the domain owns the rules. Each side
can change without dragging the other.

---

## How to use this list

- **In code review.** When you see the symptom, link to the section. The
  substitute is named with a concrete API path; reviewers do not have to
  guess.
- **In design discussions.** When weighing convenience against the patterns
  here, default to the substitute. The shorthand version always seems
  cheaper until the second bug.
- **When extending core.** Any new API in `go-ddd-core` should make at
  least one of these substitutes easier to adopt, not the anti-pattern.
