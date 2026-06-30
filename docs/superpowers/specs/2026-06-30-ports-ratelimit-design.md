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
	// RetryAfter MUST be present. When Allowed it MUST be 0. When !Allowed it is
	// a conservative wait hint: it MUST NOT be less than the limiter's known
	// earliest-retry time (under-estimating is a bug), and MAY be larger (a
	// conservative over-estimate is fine — the client just waits longer).
	// Retrying before it elapses carries NO success guarantee; it is NOT a
	// guarantee of denial. Per IETF draft-ietf-httpapi-ratelimit-headers-11,
	// reset/retry timing is a hint (a server MAY alter quota between requests),
	// not a hard "denied until then" promise. Reports on the safe side like jobs
	// Job.ProcessAt's "no earlier than", but without ProcessAt's hard guarantee.
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
	//
	// Multi-policy projection: a limiter MAY enforce several quota policies at
	// once (IETF draft-11 RateLimit-Policy is a list, e.g. 50/60s AND
	// 1000/3600s). Result carries at most ONE policy's metadata, so the adapter
	// MUST project the policy that BOUND this decision — on a denial, the policy
	// that denied; on an allow, the most-constraining policy it can honestly
	// represent. If no single policy is a faithful representative, the fields
	// MUST be absent rather than a fabricated blend. Surfacing every policy is an
	// adapter-layer concern, outside this single-Result contract.
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
	// (with a RetryAfter wait hint), nil. The Limiter NEVER returns
	// errorsx.CodeRateLimited for ordinary denial. Because Allowed==false is data
	// (there is no error value), HTTP middleware SHOULD emit 429 directly from the
	// Allowed==false decision rather than route it through an error-translation
	// pipeline (httpx.Translate takes an error, which this is not); only if a
	// particular transport pipeline requires an error object should it mint a
	// CodeRateLimited error inside that adapter.
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
| `RetryAfter` | MUST present;`Allowed`→0;`!Allowed`→**保守等待 hint**:不低於 limiter 已知最早可重試時間(低估是 bug)、可高估;在它之前 retry **無成功保證**(非保證 denied,對齊 IETF draft-11「reset 是 hint 非硬保證」) |
| `Limit`/`Remaining`/`ResetAt` | **accurate-or-absent**(真值含演算法固有精度 or sentinel,禁捏造)+ **advisory-only**(consumer 只發 header、MUST NOT 用於 allow/deny);`!HasX()`/`IsZero()` 時 consumer MUST omit header(不得序列化 `UnknownCount`) |

**overflow 規則**:當 upstream policy / HTTP header 表達的數值超出本機 `int` 可靠範圍,adapter MUST 將該欄位設為 `UnknownCount`(absent),**不得截斷或飽和成假值** —— 這是 accurate-or-absent 的延伸(假值即捏造)。

**denial 不走 error channel**:正常限流 MUST 回 `Result{Allowed:false}, nil`。`errorsx.CodeRateLimited`(已存在於 `pkg/errorsx`,`StatusFromCode` 映射 429)**Limiter 永不回傳**。因 `Allowed==false` 是 data(無 error 物件),HTTP middleware 應**直接從 `Allowed==false` 發 429**,不要走 `httpx.Translate(err)` 這類 error pipeline;僅當某 transport pipeline 硬需 error 物件時,才在該 adapter 內部自造 `CodeRateLimited` error。

**多 policy 投影**:limiter 可能同時套用多組 quota policy(IETF draft-11 `RateLimit-Policy` 是 list,如 50/60s AND 1000/3600s)。`Result` 只承載**一組** metadata,故 adapter MUST 投影**約束本次決策的那個 policy**(被拒時=觸發拒絕的 policy;放行時=能誠實表示的最受限 policy);若無單一 policy 可忠實代表 → 該欄 absent,不得折疊成捏造的混合值。完整多 policy 表達是 adapter 層的事。

## 4. error 語意(沿用 jobs 兩類 + precedence)

- **限流決策不是 error** → `Result{Allowed:false}, nil`
- **CLASS 1 validation**:空 key → `errorsx.CodeInvalidArgument`(deterministic,definitely 未計數)
  - 空 key 不是「匿名」,而是 caller 沒建好 partition key;放過會把不同 caller 打到同一個 bucket —— 這是 contract bug,fail-loud(對齊 `jobs.Type` 空字串紀律)
- **CLASS 2**:ctx 已取消/逾期 → `errors.Is(context.Canceled/DeadlineExceeded)`(無 backend 接觸);backend 失敗 → coded `errorsx`(`CodeOf != CodeUnknown`:不可達 `CodeUnavailable`、無法分類 `CodeInternal`)
- **precedence**:空 key → pre-cancelled/expired ctx → backend(與 `jobs.Enqueue` 同構)

## 5. `ratelimittest` conformance suite(只測不依賴 wall-clock 的不變式)

對齊 `jobstest`/`idempotencytest` 的「只出可確定性測試那半」紀律。`RunContract` 接受 factory,**每個 subtest 取一個 fresh、isolated limiter**(對齊 jobstest/idempotencytest,避免 suite 間狀態污染),要求配置成 deterministic profile(同 key 第 1 次 allowed、第 2 次 denied)。

**測:**
- 空 key → `CodeInvalidArgument` + precedence
- pre-cancelled / pre-expired ctx → `errors.Is`
- same-key depletion(不睡覺連打,第 2 次 `Allowed=false, err=nil`)
- key isolation(A 耗盡不影響 B 第一次 `Allow`)
- **`RetryAfter` invariant**:`Allowed`→`RetryAfter==0`;`!Allowed && err==nil`→`RetryAfter>0`(擋掉 `Result{}, nil` 的 zero-value 退化:denied 卻無等待 hint)
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
4. **`RetryAfter` 用保守 hint 語意**(而非精確值,也非「之前保證 denied」的 floor)—— 近似/分散式演算法能誠實滿足「不低於最早可重試」的保守上估,但無法滿足精確值;且 IETF draft-11 明示 reset/retry 是 hint、server 可隨請求調整 quota,故不宣稱「之前一定被拒」。保守 hint 讓 `RetryAfter` 留在「全演算法可 conform」這側。
5. **空 key → `CodeInvalidArgument`** —— fail-loud;空 key 是 caller 漏建 partition key,silent 共用一桶是最難查的限流 bug。
6. **不綁死 header spelling** —— IETF rate-limit header spec 仍是 Internet-Draft(`draft-ietf-httpapi-ratelimit-headers-11`,2026-05-23,會演化);doc 只描述語意(`Retry-After` + quota hints),不寫死 `RateLimit-*` 欄位名或 spec 版本。
   - 已查證 draft-11(WebFetch,2026-06-30):支援 **multiple quota policies**(`RateLimit-Policy` 為 list)→ 催生 §3 多 policy 投影規則;reset/retry 時間是 **hint 非硬保證**(原文「Clients MUST NOT assume that a positive available quota is a guarantee that further requests will be served」)→ 修正 `RetryAfter` 語意;metadata 為 **advisory hints**(「MUST NOT consider the available quota parameter as a service level agreement」)→ 支撐 advisory-only 義務。
7. **單一 `Result` + 投影 binding policy**(而非 `Result` 攜帶 policy 陣列)—— core 保持單組 metadata 的最小表面積;多 policy 並存時投影「約束本次決策」的那組、無單一忠實代表則 absent。完整 policy 陣列表達(若某 service 需要)是 adapter 層擴充,不汙染 core 契約。
