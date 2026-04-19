// Package account is the worked example for docs/aggregate-design.md. It
// demonstrates a rich aggregate with multiple invariants per method, value
// objects expressed as enum-like constants, and domain events recorded at the
// point of mutation.
//
// Compared with the Order aggregate next door, Account exercises:
//   - Multi-rule methods (Withdraw enforces status, amount sign, and overdraft)
//   - Status-gated mutations (Freeze only from Open; Withdraw blocked when Frozen)
//   - A constructor that itself enforces an invariant (non-negative opening deposit)
package account

import (
	"github.com/slam0504/go-ddd-core/domain"
)

// ID is the aggregate identifier.
type ID string

// Status enumerates the account lifecycle.
type Status string

const (
	StatusOpen   Status = "open"
	StatusFrozen Status = "frozen"
	StatusClosed Status = "closed"
)

// Account is the aggregate root. Public surface is intent-named methods —
// no SetStatus, no SetBalance — so every state change flows through a rule
// check and a recorded event.
type Account struct {
	domain.BaseAggregate[ID]
	status    Status
	balance   int64 // cents
	overdraft int64 // cents allowed below zero (0 means strict)
}

// New opens a fresh account with an opening deposit. Returns a RuleViolation
// when the deposit is negative; this is the only invariant that can be
// violated at construction time.
func New(id ID, openingDeposit int64, eventID string) (*Account, error) {
	if openingDeposit < 0 {
		return nil, domain.NewRuleViolation("OPENING_DEPOSIT_NEGATIVE",
			"opening deposit must be non-negative")
	}
	a := &Account{
		BaseAggregate: domain.NewBaseAggregate[ID](id),
		status:        StatusOpen,
		balance:       openingDeposit,
	}
	a.IncrementVersion()
	a.Record(NewAccountOpened(eventID, string(id), a.Version(), openingDeposit))
	return a, nil
}

// SetOverdraftLimit configures how far the balance may go below zero.
// Returns a RuleViolation if the limit is negative.
func (a *Account) SetOverdraftLimit(limit int64, eventID string) error {
	if limit < 0 {
		return domain.NewRuleViolation("OVERDRAFT_NEGATIVE",
			"overdraft limit must be non-negative")
	}
	a.overdraft = limit
	a.IncrementVersion()
	a.Record(NewOverdraftLimitChanged(eventID, string(a.ID()), a.Version(), limit))
	return nil
}

// Withdraw debits amount from the balance. Enforces (in order):
//
//  1. The account is not frozen or closed.
//  2. Amount is positive.
//  3. The post-withdrawal balance does not fall below the overdraft limit.
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

// Deposit credits amount to the balance. Allowed on Open and Frozen accounts
// (a frozen account can still receive money — it just cannot pay out); blocked
// on Closed.
func (a *Account) Deposit(amount int64, eventID string) error {
	if a.status == StatusClosed {
		return domain.NewRuleViolation("ACCOUNT_CLOSED",
			"deposits are not allowed on closed accounts")
	}
	if amount <= 0 {
		return domain.NewRuleViolation("AMOUNT_NOT_POSITIVE",
			"deposit amount must be positive")
	}
	a.balance += amount
	a.IncrementVersion()
	a.Record(NewMoneyDeposited(eventID, string(a.ID()), a.Version(), amount, a.balance))
	return nil
}

// Freeze transitions an Open account to Frozen.
func (a *Account) Freeze(reason string, eventID string) error {
	if a.status != StatusOpen {
		return domain.NewRuleViolation("ONLY_OPEN_CAN_FREEZE",
			"only open accounts can be frozen")
	}
	if reason == "" {
		return domain.NewRuleViolation("FREEZE_REASON_REQUIRED",
			"freeze reason is mandatory")
	}
	a.status = StatusFrozen
	a.IncrementVersion()
	a.Record(NewAccountFrozen(eventID, string(a.ID()), a.Version(), reason))
	return nil
}

// Close transitions any non-closed account to Closed. Closing is final;
// the only path out is opening a new account.
func (a *Account) Close(eventID string) error {
	if a.status == StatusClosed {
		return domain.NewRuleViolation("ALREADY_CLOSED",
			"account is already closed")
	}
	a.status = StatusClosed
	a.IncrementVersion()
	a.Record(NewAccountClosed(eventID, string(a.ID()), a.Version()))
	return nil
}

// Read methods. Kept minimal — every getter is a chance for a caller to
// bypass the aggregate's rules.
func (a *Account) Status() Status   { return a.status }
func (a *Account) Balance() int64   { return a.balance }
func (a *Account) Overdraft() int64 { return a.overdraft }
