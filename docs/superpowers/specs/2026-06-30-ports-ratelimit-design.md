# `ports/ratelimit` 契約設計

- **狀態**:設計定案,待實作(writing-plans)
- **日期**:2026-06-30
- **Branch**:`feat/ports-ratelimit`
- **四象限定位**:A. Core contract gap —— `docs/roadmap.md` A 象限的最後一項
- **前序 cycle**:health(v0.5.0)→ AuthN(v0.6.0)→ AuthZ(v0.7.0)→ idempotency(v0.8.0)→ jobs(v0.9.0)

## 1. 目標與範圍

定義 **inbound 請求節流**契約。`Limiter` 回答單一問題:「**這個 key 現在可不可以進行?**」決策以 **data**(`Result.Allowed`)回傳,**永不**用 error 表達正常限流。同時攜帶 advisory quota metadata,讓 HTTP transport adapter 可選擇性地發出 rate-limit 回應提示(`Retry-After` 以及該服務 HTTP 慣例採用的 quota header)。

Core **不**固定節流演算法(token-bucket / sliding-window / GCRA…)、rate/burst/window 配置、儲存後端 —— 全部在 adapter。

### 刻意外置(scope-out)

| 項目 | 理由 |
|---|---|
| outbound quota / blocking `Wait`·`Reserve` | 由 httpclient consumer 驅動,屆時依實證決定形狀;不在 inbound 契約預先臆測 |
| `AllowN` / cost(一次扣 N) | YAGNI —— inbound 幾乎都是 1 req=1 token,加權需求無 consumer 證據 |
| `LimiterFunc`(Func adapter) | limiter 本質有狀態(要存 counter),無狀態 Func adapter 無用武之地;與 `health.NewCheck`/`jobs.HandlerFunc`(wrap 無狀態函式)情況不同 |
| 精確 timing 語意 | refill 時機、reset 精準時刻、`RetryAfter` 是否遞減、`ResetAt` 是否對齊 wall-clock —— 留 adapter,且非可確定性驗證對象 |

## 2. 核心型別

```go
// Package ratelimit defines the inbound request-throttling contract. A Limiter
// answers a single question — "may this key proceed right now?" — and returns
// the decision as data (Result.Allowed), never as an error. It also carries
// advisory quota metadata (limit / remaining / reset) that an HTTP transport
// adapter MAY surface as rate-limit response hints — Retry-After plus whatever
// quota headers the service's HTTP convention uses. Core fixes neither the
// throttling algorithm (token-bucket, sliding-window, GCRA, …), the
// rate/burst/window configuration, nor the storage backend — all live in
// adapters.
//
// Every ctx-taking method requires a non-nil ctx (stdlib convention).
package ratelimit

import (
	"context"
	"time"
)

// UnknownCount marks Limit/Remaining as absent — the limiter cannot honestly
// produce the value (e.g. a token-bucket has no discrete window, or an upstream
// quota exceeds what an int can hold). It is distinct from a real 0. Mirrors
// pkg/pagination.Page.Total's -1 "not computed" sentinel.
const UnknownCount = -1

// Result is the outcome of one Allow call. Allowed is the decision; the rest is
// advisory metadata for response headers (see field docs for each obligation).
type Result struct {
	// Allowed MUST be exact — it is the decision itself.
	Allowed bool
	// RetryAfter MUST be present. When Allowed it MUST be 0. When !Allowed it
	// MUST be a floor: a "no sooner than" lower bound — retrying before it
	// elapses is guaranteed still denied. Over-estimating is safe (client waits
	// longer); under-estimating is a bug. Same single-direction floor as jobs
	// Job.ProcessAt.
	RetryAfter time.Duration
	// Limit / Remaining / ResetAt are accurate-or-absent advisory metadata:
	// each is either a value the limiter genuinely computed (carrying that
	// algorithm's inherent precision; a distributed limiter's stale value
	// counts) or a defined "absent" sentinel — NEVER a fabricated placeholder.
	// They are advisory-only: a consumer MAY surface them as headers but MUST
	// NOT use them to make its own allow/deny decision (that is Allowed's job;
	// Remaining is TOCTOU-stale the instant it is read). A consumer MUST omit
	// the corresponding header when the value is absent (see HasLimit /
	// HasRemaining / ResetAt.IsZero) — it MUST NOT serialise UnknownCount.
	Limit     int       // UnknownCount means absent.
	Remaining int       // UnknownCount means absent.
	ResetAt   time.Time // IsZero means absent.
}

// HasLimit reports whether Limit carries a real value (not absent).
func (r Result) HasLimit() bool { return r.Limit >= 0 }

// HasRemaining reports whether Remaining carries a real value (not absent).
func (r Result) HasRemaining() bool { return r.Remaining >= 0 }

// Limiter decides whether a key may proceed. Implementations MUST be safe for
// concurrent use — middleware calls Allow on every inbound request.
type Limiter interface {
	// Allow reports whether key may proceed right now, as Result data. Ordinary
	// quota exhaustion is NOT an error: it MUST return Result{Allowed:false}
	// (with a RetryAfter floor), nil. errorsx.CodeRateLimited is NOT used for
	// ordinary denial — it exists only so a transport adapter can translate an
	// Allowed==false decision into HTTP 429; the Limiter itself never returns it.
	//
	// A non-nil error means the limiter could not reach a decision, in two
	// classes with fixed precedence (same shape as jobs.Enqueuer):
	//   CLASS 1 — validation: an empty key is malformed input (a missing
	//     partition key, not an "anonymous" caller) → errorsx.CodeInvalidArgument;
	//     deterministic, nothing consumed.
	//   CLASS 2 — ctx / backend: a ctx already cancelled or past its deadline at
	//     entry returns the matching ctx error (errors.Is context.Canceled /
	//     context.DeadlineExceeded) with NO backend contact; otherwise a backend
	//     failure is a coded errorsx whose CodeOf is NOT CodeUnknown
	//     (unreachable → CodeUnavailable; unclassifiable → CodeInternal).
	//   Precedence: empty key → pre-cancelled/expired ctx → backend.
	Allow(ctx context.Context, key string) (Result, error)
}
```

## 3. 欄位義務(契約核心)

| 欄位 | 義務 |
|---|---|
| `Allowed` | MUST 精確 —— 決策本身 |
| `RetryAfter` | MUST present;`Allowed`→0;`!Allowed`→**floor**(no-sooner-than,高估安全、低估是 bug)。沿用 jobs `ProcessAt` 單向 floor |
| `Limit`/`Remaining`/`ResetAt` | **accurate-or-absent**(真值含演算法固有精度 or sentinel,禁捏造)+ **advisory-only**(consumer 只發 header、MUST NOT 用於 allow/deny);`!HasX()`/`IsZero()` 時 consumer MUST omit header(不得序列化 `UnknownCount`) |

**overflow 規則**:當 upstream policy / HTTP header 表達的數值超出本機 `int` 可靠範圍,adapter MUST 將該欄位設為 `UnknownCount`(absent),**不得截斷或飽和成假值** —— 這是 accurate-or-absent 的延伸(假值即捏造)。

**denial 不走 error channel**:正常限流 MUST 回 `Result{Allowed:false}, nil`;`errorsx.CodeRateLimited`(已存在於 `pkg/errorsx`,`StatusFromCode` 映射 429)**不用於** ordinary limiter denial,僅供 transport adapter 把 `Allowed==false` 翻成 HTTP 429。明文寫入 doc,避免 adapter 作者照名字回 error。

## 4. error 語意(沿用 jobs 兩類 + precedence)

- **限流決策不是 error** → `Result{Allowed:false}, nil`
- **CLASS 1 validation**:空 key → `errorsx.CodeInvalidArgument`(deterministic,definitely 未計數)
  - 空 key 不是「匿名」,而是 caller 沒建好 partition key;放過會把不同 caller 打到同一個 bucket —— 這是 contract bug,fail-loud(對齊 `jobs.Type` 空字串紀律)
- **CLASS 2**:ctx 已取消/逾期 → `errors.Is(context.Canceled/DeadlineExceeded)`(無 backend 接觸);backend 失敗 → coded `errorsx`(`CodeOf != CodeUnknown`:不可達 `CodeUnavailable`、無法分類 `CodeInternal`)
- **precedence**:空 key → pre-cancelled/expired ctx → backend(與 `jobs.Enqueue` 同構)

## 5. `ratelimittest` conformance suite(只測不依賴 wall-clock 的不變式)

對齊 `jobstest`/`idempotencytest` 的「只出可確定性測試那半」紀律。`RunContract` 接受 factory,要求配置成 deterministic profile(同 key 第 1 次 allowed、第 2 次 denied)。

**測:**
- 空 key → `CodeInvalidArgument` + precedence
- pre-cancelled / pre-expired ctx → `errors.Is`
- same-key depletion(不睡覺連打,第 2 次 `Allowed=false, err=nil`)
- key isolation(A 耗盡不影響 B 第一次 `Allow`)
- **`RetryAfter` invariant**:`Allowed`→`RetryAfter==0`;`!Allowed && err==nil`→`RetryAfter>0`(擋掉 `Result{}, nil` 的 zero-value 退化:denied 卻無 no-sooner-than floor)
- metadata shape:`Limit`/`Remaining` 僅 `UnknownCount` 或非負;均 known 時 `Remaining≤Limit`;`Remaining==0` 視為 known(非 absent)
- denied-is-data-not-error(正常限流不得回 error)
- `HasLimit`/`HasRemaining` 對 `UnknownCount`/`0` 的 executable spec

**不測:**refill 時機 / reset 精準時刻 / `RetryAfter` 是否遞減 / `ResetAt` 對齊 wall-clock / 演算法細節 / 高併發精準允許數(除非 contract 另要求 adapter 提供測試 profile)

## 6. 檔案組織(對齊 jobs/idempotency)

```
ports/ratelimit/ratelimit.go                        契約 + Result + Limiter + package doc
ports/ratelimit/ratelimit_test.go                   Result/HasX 單元 + UnknownCount round-trip
ports/ratelimit/ratelimittest/ratelimittest.go      RunContract
ports/ratelimit/ratelimittest/ratelimittest_test.go in-package reference limiter 跑通 RunContract
```

## 7. 驗證策略

- `gofmt -l ports/`(無輸出)
- `go build ./...`
- `go vet ./...`
- `go test ./ports/ratelimit/...`(含 reference limiter 跑通 `RunContract`)

## 8. 設計決策記錄(關鍵取捨)

1. **介面回 `Result` 而非裸 bool** —— inbound 場景需要 `Retry-After` + quota hints;裸 bool 會逼每個 middleware 另開 side channel 搬 header 資料,抵銷「出一個 core 契約」的意義。
2. **`Result` 用扁平 value struct + `-1`/`IsZero` sentinel + `HasX` 謂詞**,不用 `*int` 或 accessor-only —— 對齊 `pkg/pagination.Page.Total` 的既有 `-1` 先例與 repo dominant 的 value-struct 風格;metadata 是 advisory 而非授權/資安不變式,不值得付 constructor/accessor 稅。
3. **accurate-or-absent + advisory-only**(而非「best-effort」)—— 「best-effort」是契約氣味,會誘 adapter 塞近似垃圾、consumer 拿去當真;「禁捏造 + 只發 header + 不可決策」才是可驗證義務,且讓全部演算法(含分散式近似)可 conform。
4. **`RetryAfter` 用 floor 語意**(而非精確值)—— 近似/分散式演算法能誠實滿足下界但無法滿足精確值;floor 讓 `RetryAfter` 留在「全演算法可 conform」這側。
5. **空 key → `CodeInvalidArgument`** —— fail-loud;空 key 是 caller 漏建 partition key,silent 共用一桶是最難查的限流 bug。
6. **不綁死 header spelling** —— IETF rate-limit header spec 仍是 Internet-Draft(會演化);doc 只描述語意(`Retry-After` + quota hints),不寫死 `RateLimit-*` 欄位名或 spec 版本。
   - 註:draft 現況(以 `RateLimit`/`RateLimit-Policy` 為中心)未在本次獨立查證;但「core 不綁 header spelling」無論 draft 如何演化都成立,故此決策不依賴該查證。
