package account_test

import (
	"errors"
	"testing"

	"github.com/slam0504/go-ddd-core/domain"
	"github.com/slam0504/go-ddd-core/examples/minimal/domain/account"
)

// helper: assert that err is a RuleViolation with the given code.
func assertRule(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected RuleViolation %q, got nil", wantCode)
	}
	var rv *domain.RuleViolation
	if !errors.As(err, &rv) {
		t.Fatalf("expected *domain.RuleViolation, got %T: %v", err, err)
	}
	if rv.Code != wantCode {
		t.Fatalf("rule code = %q, want %q (msg: %s)", rv.Code, wantCode, rv.Message)
	}
}

// helper: open a fresh account and clear the opening event so per-test
// assertions on DomainEvents() see only the events the test triggered.
func newOpenAccount(t *testing.T, deposit int64) *account.Account {
	t.Helper()
	a, err := account.New("acct-1", deposit, "evt-open")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a.ClearEvents()
	return a
}

// --- New ---

func TestNew_HappyPathRecordsAccountOpened(t *testing.T) {
	a, err := account.New("acct-1", 100, "evt-1")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.Status() != account.StatusOpen {
		t.Fatalf("status = %q, want open", a.Status())
	}
	if a.Balance() != 100 {
		t.Fatalf("balance = %d, want 100", a.Balance())
	}
	if a.Version() != 1 {
		t.Fatalf("version = %d, want 1", a.Version())
	}
	events := a.DomainEvents()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	opened, ok := events[0].(account.AccountOpened)
	if !ok {
		t.Fatalf("event type = %T, want AccountOpened", events[0])
	}
	if opened.OpeningDeposit != 100 {
		t.Fatalf("opening deposit = %d, want 100", opened.OpeningDeposit)
	}
}

func TestNew_NegativeDepositRejected(t *testing.T) {
	_, err := account.New("acct-1", -1, "evt-1")
	assertRule(t, err, "OPENING_DEPOSIT_NEGATIVE")
}

// --- Withdraw ---

func TestWithdraw_HappyPath(t *testing.T) {
	a := newOpenAccount(t, 1000)
	if err := a.Withdraw(300, "evt-w"); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if a.Balance() != 700 {
		t.Fatalf("balance = %d, want 700", a.Balance())
	}
	events := a.DomainEvents()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	w, ok := events[0].(account.MoneyWithdrawn)
	if !ok {
		t.Fatalf("event type = %T, want MoneyWithdrawn", events[0])
	}
	if w.Amount != 300 || w.BalanceAfter != 700 {
		t.Fatalf("event = %+v", w)
	}
}

func TestWithdraw_BlockedWhenFrozen(t *testing.T) {
	a := newOpenAccount(t, 1000)
	if err := a.Freeze("audit", "evt-f"); err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	a.ClearEvents()
	err := a.Withdraw(100, "evt-w")
	assertRule(t, err, "ACCOUNT_FROZEN")
	if a.Balance() != 1000 {
		t.Fatalf("balance must not change on rule violation, got %d", a.Balance())
	}
	if len(a.DomainEvents()) != 0 {
		t.Fatalf("no event should be recorded on rule violation")
	}
}

func TestWithdraw_BlockedWhenClosed(t *testing.T) {
	a := newOpenAccount(t, 1000)
	if err := a.Close("evt-c"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := a.Withdraw(100, "evt-w")
	assertRule(t, err, "ACCOUNT_CLOSED")
}

func TestWithdraw_NonPositiveAmountRejected(t *testing.T) {
	a := newOpenAccount(t, 1000)
	for _, amt := range []int64{0, -50} {
		err := a.Withdraw(amt, "evt-w")
		assertRule(t, err, "AMOUNT_NOT_POSITIVE")
	}
}

func TestWithdraw_OverdraftRespected(t *testing.T) {
	a := newOpenAccount(t, 100)
	if err := a.SetOverdraftLimit(50, "evt-o"); err != nil {
		t.Fatalf("SetOverdraftLimit: %v", err)
	}
	a.ClearEvents()

	// 100 + 50 overdraft = 150 spendable. 150 succeeds.
	if err := a.Withdraw(150, "evt-w1"); err != nil {
		t.Fatalf("Withdraw 150: %v", err)
	}
	if a.Balance() != -50 {
		t.Fatalf("balance = %d, want -50", a.Balance())
	}

	// One more cent should now exceed the overdraft.
	err := a.Withdraw(1, "evt-w2")
	assertRule(t, err, "OVERDRAFT_EXCEEDED")
}

// --- Deposit ---

func TestDeposit_HappyPath(t *testing.T) {
	a := newOpenAccount(t, 100)
	if err := a.Deposit(50, "evt-d"); err != nil {
		t.Fatalf("Deposit: %v", err)
	}
	if a.Balance() != 150 {
		t.Fatalf("balance = %d, want 150", a.Balance())
	}
}

func TestDeposit_AllowedWhenFrozen(t *testing.T) {
	// A frozen account cannot pay out, but it should still receive money —
	// otherwise legitimate refunds and reversals would be blocked.
	a := newOpenAccount(t, 100)
	if err := a.Freeze("audit", "evt-f"); err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if err := a.Deposit(50, "evt-d"); err != nil {
		t.Fatalf("Deposit on frozen account should be allowed, got %v", err)
	}
	if a.Balance() != 150 {
		t.Fatalf("balance = %d, want 150", a.Balance())
	}
}

func TestDeposit_BlockedWhenClosed(t *testing.T) {
	a := newOpenAccount(t, 100)
	if err := a.Close("evt-c"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := a.Deposit(50, "evt-d")
	assertRule(t, err, "ACCOUNT_CLOSED")
}

func TestDeposit_NonPositiveAmountRejected(t *testing.T) {
	a := newOpenAccount(t, 100)
	for _, amt := range []int64{0, -10} {
		err := a.Deposit(amt, "evt-d")
		assertRule(t, err, "AMOUNT_NOT_POSITIVE")
	}
}

// --- Freeze ---

func TestFreeze_HappyPath(t *testing.T) {
	a := newOpenAccount(t, 100)
	if err := a.Freeze("audit", "evt-f"); err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if a.Status() != account.StatusFrozen {
		t.Fatalf("status = %q, want frozen", a.Status())
	}
}

func TestFreeze_OnlyFromOpen(t *testing.T) {
	a := newOpenAccount(t, 100)
	if err := a.Freeze("audit", "evt-f1"); err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	err := a.Freeze("again", "evt-f2")
	assertRule(t, err, "ONLY_OPEN_CAN_FREEZE")
}

func TestFreeze_RequiresReason(t *testing.T) {
	a := newOpenAccount(t, 100)
	err := a.Freeze("", "evt-f")
	assertRule(t, err, "FREEZE_REASON_REQUIRED")
}

// --- Close ---

func TestClose_HappyPath(t *testing.T) {
	a := newOpenAccount(t, 100)
	if err := a.Close("evt-c"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if a.Status() != account.StatusClosed {
		t.Fatalf("status = %q, want closed", a.Status())
	}
}

func TestClose_IsIdempotentlyRejected(t *testing.T) {
	a := newOpenAccount(t, 100)
	if err := a.Close("evt-c1"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := a.Close("evt-c2")
	assertRule(t, err, "ALREADY_CLOSED")
}

// --- SetOverdraftLimit ---

func TestSetOverdraftLimit_HappyPath(t *testing.T) {
	a := newOpenAccount(t, 100)
	if err := a.SetOverdraftLimit(200, "evt-o"); err != nil {
		t.Fatalf("SetOverdraftLimit: %v", err)
	}
	if a.Overdraft() != 200 {
		t.Fatalf("overdraft = %d, want 200", a.Overdraft())
	}
}

func TestSetOverdraftLimit_NegativeRejected(t *testing.T) {
	a := newOpenAccount(t, 100)
	err := a.SetOverdraftLimit(-1, "evt-o")
	assertRule(t, err, "OVERDRAFT_NEGATIVE")
}

// --- Version & event sequencing ---

func TestVersionIncrementsPerSuccessfulMutation(t *testing.T) {
	a, err := account.New("acct-1", 100, "evt-open")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.Version() != 1 {
		t.Fatalf("after New version = %d, want 1", a.Version())
	}
	if err := a.Deposit(50, "evt-d"); err != nil {
		t.Fatalf("Deposit: %v", err)
	}
	if a.Version() != 2 {
		t.Fatalf("after Deposit version = %d, want 2", a.Version())
	}
	if err := a.Withdraw(20, "evt-w"); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if a.Version() != 3 {
		t.Fatalf("after Withdraw version = %d, want 3", a.Version())
	}
}

func TestVersionDoesNotChangeOnRuleViolation(t *testing.T) {
	a := newOpenAccount(t, 100)
	beforeVersion := a.Version()
	_ = a.Withdraw(-1, "evt-w") // rule violation
	if a.Version() != beforeVersion {
		t.Fatalf("version changed on rule violation: %d → %d", beforeVersion, a.Version())
	}
	if len(a.DomainEvents()) != 0 {
		t.Fatalf("event recorded on rule violation")
	}
}
