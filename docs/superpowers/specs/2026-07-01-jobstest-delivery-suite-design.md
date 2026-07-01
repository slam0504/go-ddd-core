# `jobstest` delivery/timing conformance suite 設計

- **狀態**:設計定案,待實作(writing-plans)
- **日期**:2026-07-01
- **Branch**:`feat/jobstest-delivery-suite`
- **緣起**:`.agent/decisions.md`「after the first adapter lands, distill the delivery/timing half of `jobstest` from the proven common semantics」。首個 jobs adapter `jobs/asynq` 已於 v0.9.0 落地並通過 (0)+(a)–(v),提煉封鎖已解除。

## 1. 目標與範圍

`jobstest.RunContract` 目前**刻意只出 synchronous 半**(never runs a worker)。本設計新增 exported `jobstest.RunDeliveryContract`,把 `jobs/asynq` 已實證、**且純透過 `jobs.Enqueuer`/`jobs.Worker`/handler observation 就能證明**的 delivery/timing 不變式提煉成可重用 suite,讓未來的 jobs adapter(River、SQL queue)不必各自重寫這批測試。

### 納入(介面可觀察,11 個 subtest)

只收「可透過 port 介面與 handler observation 證明」的 common semantics。錨定於 `go-ddd-adapters` 的 `jobs/asynq` `delivery_test.go` + `shutdown_test.go`(integration build tag)已證明的機制。

### 刻意排除(需 backend 內省 / 故障注入,留 adapter tag-gate)

`(g)` write-nothing、`(t)` accepted-but-ack-lost、`(f)` unreachable backend、`(h)` fatal startup、`(n)` retention 的 inspection 版、`(s)` recoverable-state classification(completed/pendingRetryable/activeLeased/lostDiscarded)、`(u)` unhandled-type final policy、requeue/ack-race state。

**理由(比例原則)**:這些需要 adapter-specific introspection(asynq 用 `Inspector`/`TaskState*`)或 fault injection(go-redis hook)。目前只有 Asynq 一個 production adapter 證明過 (a)–(v);在沒有第二個實作(River/SQL)佐證「observation model 是跨 adapter 穩定形狀」前,現在定義 `Introspector` 抽象會太早、且易把 Asynq 的 task-state taxonomy 包成假通用介面。**判準**:待第二個 production adapter 落地後,若兩邊自然收斂出同一 introspection 形狀,再升級為 opt-in 的 `RunRecoverabilityContract(t, factory, observer)`。現在不做。

## 2. API 與 fixture 型別

新檔 `ports/jobs/jobstest/delivery.go`,**同 `jobstest` package**(重用既有型別慣例)。

```go
// DeliveryBounds are the adapter-declared timing bounds the delivery suite
// waits on. All are REQUIRED; a non-positive value is a t.Fatalf (there is no
// core default — a bound is the adapter's own promise, not something core can
// guess). Per the ports/jobs (v) single-source rule, each MUST be the adapter's
// own exported constant/option value.
type DeliveryBounds struct {
	// ShutdownWithin bounds Run returning nil after its ctx is cancelled.
	ShutdownWithin time.Duration
	// DeliverWithin bounds a freshly-eligible job reaching its handler.
	DeliverWithin time.Duration
	// RedeliverWithin bounds a failed handler attempt being redelivered.
	RedeliverWithin time.Duration
	// ProcessAtDelay is the future offset the suite uses for not-before-ProcessAt
	// tests: the adapter declares "my scheduler can be stably tested at this
	// future delay." The suite schedules ProcessAt = now + ProcessAtDelay, asserts
	// no delivery before it, and eventual delivery within ProcessAtDelay +
	// DeliverWithin. Not a core default — the adapter's declared scheduling knob.
	// The declared value MUST cover the scheduler granularity, the worker poll
	// interval, and the expected backend-clock skew in the test environment; a
	// too-small value makes the not-before assertion flaky.
	ProcessAtDelay time.Duration
}

// DeliveryFixture is one isolated backing store for a delivery subtest: an
// Enqueuer plus a factory for Workers over that SAME store (delivery invariants
// need more than one Worker instance — see NewWorker), plus the declared bounds.
type DeliveryFixture struct {
	// Enqueuer submits jobs to the shared backing store.
	Enqueuer jobs.Enqueuer
	// NewWorker returns a fresh Worker over the SAME backing store as Enqueuer.
	// Worker.Run is once-per-instance, so tests that stop one Worker and expect a
	// new instance to deliver (criterion r) need to spawn a second Worker over the
	// same store; a single Backend.Worker cannot express that.
	NewWorker func() jobs.Worker
	// Bounds are this fixture's declared timing bounds.
	Bounds DeliveryBounds
}

// DeliveryFactory returns a FRESH, ISOLATED DeliveryFixture for one subtest: a
// backing store with no jobs and no registrations shared with any other call.
// Register cleanup via t.Cleanup.
type DeliveryFactory func(t *testing.T) DeliveryFixture

// RunDeliveryContract runs the delivery/timing conformance suite against a real,
// running Worker. Unlike RunContract (synchronous-only), it starts Workers, waits
// on the fixture's declared bounds, and asserts only what is observable through
// jobs.Enqueuer / jobs.Worker / a test handler — no backend introspection, no
// fault injection. Recoverability, retention, and fault-classification invariants
// need adapter-specific observation and stay in the adapter's own tag-gate tests.
func RunDeliveryContract(t *testing.T, factory DeliveryFactory)
```

**與先前 `{Backend, Bounds}` 的差異**:改用 `NewWorker func() jobs.Worker` 取代單一 `Worker`,因 (r) 及 stopped-worker 類測試需要在同一 store 上建第二個 Worker 實例(`Run` 是 once-per-instance)。

## 3. Fixture preconditions(adapter 責任,寫進 doc)

1. **isolation**:每次 `factory(t)` 回一個**隔離**的 backing store(無共享 jobs / registrations)。
2. **shared store**:`NewWorker()` 造的每個 Worker 都 over `Enqueuer` 的**同一** backing store。
3. **retry-enabled**:*The fixture MUST configure failed handler attempts to be redelivered within `RedeliverWithin`; the suite assumes at least one retry attempt and a retry delay short enough for the declared bound.* 這避免某 adapter 的 dead-letter / no-retry 預設被誤判成 contract failure((a)/(d)/(e) 的前提;asynq 靠 `WithMaxRetry>=1` + 短 `WithRetryDelay`,suite 不能設這種 backend 專屬 option)。

## 4. Subtests(11,全介面可觀察)

每個 subtest 呼叫 `factory(t)` 取 fresh fixture;handler 用 buffered channel 發訊號;suite `select` 等該 fixture 宣告的 bound,逾時 `t.Fatal`;每個啟動的 Worker 在 teardown 都 `assertRunNilWithin(ShutdownWithin)`。

| Subtest | crit | 機制(錨定 asynq proven test) |
|---|---|---|
| `FailedAttemptRedelivered` | (a) | handler `atomic` 計數:第 1 次回 error、第 2 次 signal channel + nil。等 `RedeliverWithin`,斷言 attempts≥2。**降調**:只證「失敗後 redelivery」,不宣稱 concurrent-duplicate tolerance(那需內省)。 |
| `NotBeforeProcessAt` | (b) | `ProcessAt = now + ProcessAtDelay`;handler signal fire-time;斷言 fire-time 不 before ProcessAt;等 `ProcessAtDelay + DeliverWithin`。 |
| `PastProcessAtEligible` | (p) | `ProcessAt` 設過去;handler 在 `DeliverWithin` 內 fire。 |
| `RunReturnsNilOnCancel` | (c) | enqueue 一個 blocking-handler job:handler signal started 後 block 於 release channel(**刻意忽略 ctx** = straggler);等 started、**再** cancel、`assertRunNilWithin(ShutdownWithin)`;`t.Cleanup` 關閉 release。先讓 worker 真正 running 才證 endpoint A(running-worker cancel→nil,且 liveness 贏過 graceful drain),避免 start-then-immediate-cancel 命中 pre-cancel fast path 的 vacuous 測法。與 (j)(ctx-observing handler)互補不重複。 |
| `PayloadMutationIsolated` | (d) | 第 1 次 attempt 就地改 `task.Payload` 後 fail;第 2 次 attempt 斷言看到原始 bytes。需 retry。 |
| `IDStableAcrossRedeliveries` | (e) | 每次 attempt 收集 `task.ID`;斷言全部 == `JobInfo.ID`。需 retry。 |
| `HandlerCtxCancelledOnShutdown` | (j) | handler signal 進場後 block 於 `<-hctx.Done()`;cancel Run;斷言 handler ctx 被取消;`assertRunNilWithin`。 |
| `ExactTypeDispatchNoPrefix` | (k) | 註冊 `"t"` 的 handler;**先** enqueue exact `"t"` 確認 handler fire、drain signal;**再** enqueue prefix-鄰近型別 `"t:x"`,在 `DeliverWithin` 內確認 handler **沒有第二次** fire。exact-first 只觀察「exact handler 不收 prefix job」,不依賴 unhandled-`"t:x"` 的 adapter policy(屬刻意排除的 (u)),也避免某些 queue 的 unhandled/retry policy 阻塞後續 exact job。 |
| `DuplicateRegisterKeepsOriginal` | (o) | 註冊 h1 for `"t"`;再註冊 h2 for `"t"` → `CodeAlreadyExists`;enqueue `"t"` → h1 收、h2 不收。 |
| `NewWorkerDeliversAfterStop` | (r) | enqueue `ProcessAt = now + ProcessAtDelay`(確保 w1 取消前不 eligible);`NewWorker()` 造 w1、啟動後即 cancel、`assertRunNilWithin`;`NewWorker()` 造 w2 → 在 `ProcessAtDelay + DeliverWithin` 內 delivery。 |
| `ConcurrentEnqueueSmoke` | (l) | N goroutine 並發 `Enqueue`(-race 乾淨);cancel;`assertRunNilWithin`。**不 assert exact count**。 |

## 5. 可確定性(與 asynq 相同手法)

- handler → buffered channel signal;suite `select { case <-signal: … case <-time.After(bound): t.Fatal }`。
- 每個 Worker 的 `Run` 在 goroutine 跑,teardown `assertRunNilWithin(ShutdownWithin)`(cancel 後回 nil = endpoint A)。
- **無 introspection、無 fault injection、無硬編 sleep-then-assert**;所有等待上限來自 fixture 宣告的 bounds → 不 flaky。

## 6. 自我驗證載體

- `ports/jobs/jobstest/delivery_test.go`:實作一個 **wall-clock in-memory reference backend**(delivery 版的 ratelimit `refLimiter`)——scheduling 依 wall-clock `ProcessAt` eligibility、handler 失敗在短 `RedeliverWithin` 內 requeue、支援多 Worker 共享 store、concurrent-safe(mutex)、cancellable `Run` 在 `ShutdownWithin` 內回 nil。跑通 `RunDeliveryContract` 證明 suite 語意可被誠實兌現。
  - 註:`ports/jobs/jobs_test.go` 已有 `fakeStore`/`fakeWorker`/`fakeClock`,但用**手動 advance 的 fakeClock**;delivery suite 驅動 wall-clock,故需 wall-clock 版 reference(不能直接重用 fakeClock 版)。
- **非 vacuity 證明**:以「弱化 backend」證明 suite 有牙齒 —— 例如 no-retry backend 應讓 `FailedAttemptRedelivered` fail;ignore-ProcessAt backend 應讓 `NotBeforeProcessAt` fail。確認後不 commit 弱化 backend(同 ratelimit 的 broken-stub 手法)。

## 7. 檔案組織

```
ports/jobs/jobstest/delivery.go          RunDeliveryContract + DeliveryFixture/Factory/Bounds
ports/jobs/jobstest/delivery_test.go     wall-clock reference backend 跑通 + 非 vacuity 證明
ports/jobs/jobstest/jobstest.go          (修改)package doc:delivery-side invariants 從「NOT in this suite」→ 指向 RunDeliveryContract
```

## 8. 驗證策略

- `gofmt -l ports/jobs/`(無輸出)、`go build ./...`、`go vet ./ports/jobs/...`
- `go test ./ports/jobs/...`(含 reference backend 跑通 `RunDeliveryContract` + 既有 `RunContract` 不受影響)

## 9. 設計決策記錄(關鍵取捨)

1. **只提煉「介面可觀察」半,不出 Introspector**(scope A)—— 只有 Asynq 一個 adapter 證明過,現在抽 introspection 太早。待第二個 production adapter 落地再看是否收斂成 opt-in `RunRecoverabilityContract`。
2. **fixture 用 `NewWorker` 工廠**(而非單一 `Worker`)—— (r) 需同一 store 上第二個 Worker 實例(`Run` once-per-instance)。
3. **timing bounds(含 `ProcessAtDelay`)由 fixture 宣告、無預設**—— 對齊 (v) single-source;ProcessAt future delay 是 adapter 的 scheduler 精度承諾,非 suite 常數。
4. **retry-enabled 列為 fixture precondition**(精準措辭)—— (a)/(d)/(e) 需 redelivery;避免把 adapter 的 no-retry/dead-letter 預設誤判成 contract failure。
5. **(a) 降調為 `FailedAttemptRedelivered`**—— interface-only suite 證不了 concurrent-duplicate tolerance,不 overclaim (a) 全量。
