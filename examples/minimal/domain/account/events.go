package account

import "github.com/slam0504/go-ddd-core/domain"

// AccountOpened is emitted when an Account is created.
type AccountOpened struct {
	domain.BaseEvent
	OpeningDeposit int64
}

// NewAccountOpened constructs an AccountOpened event.
func NewAccountOpened(eventID, aggregateID string, version int64, openingDeposit int64) AccountOpened {
	return AccountOpened{
		BaseEvent:      domain.NewBaseEvent(eventID, "account.opened", aggregateID, "Account", version),
		OpeningDeposit: openingDeposit,
	}
}

// MoneyWithdrawn is emitted on each successful Withdraw call.
type MoneyWithdrawn struct {
	domain.BaseEvent
	Amount       int64
	BalanceAfter int64
}

// NewMoneyWithdrawn constructs a MoneyWithdrawn event.
func NewMoneyWithdrawn(eventID, aggregateID string, version int64, amount, balanceAfter int64) MoneyWithdrawn {
	return MoneyWithdrawn{
		BaseEvent:    domain.NewBaseEvent(eventID, "account.money_withdrawn", aggregateID, "Account", version),
		Amount:       amount,
		BalanceAfter: balanceAfter,
	}
}

// MoneyDeposited is emitted on each successful Deposit call.
type MoneyDeposited struct {
	domain.BaseEvent
	Amount       int64
	BalanceAfter int64
}

// NewMoneyDeposited constructs a MoneyDeposited event.
func NewMoneyDeposited(eventID, aggregateID string, version int64, amount, balanceAfter int64) MoneyDeposited {
	return MoneyDeposited{
		BaseEvent:    domain.NewBaseEvent(eventID, "account.money_deposited", aggregateID, "Account", version),
		Amount:       amount,
		BalanceAfter: balanceAfter,
	}
}

// AccountFrozen is emitted when an Open account transitions to Frozen.
type AccountFrozen struct {
	domain.BaseEvent
	Reason string
}

// NewAccountFrozen constructs an AccountFrozen event.
func NewAccountFrozen(eventID, aggregateID string, version int64, reason string) AccountFrozen {
	return AccountFrozen{
		BaseEvent: domain.NewBaseEvent(eventID, "account.frozen", aggregateID, "Account", version),
		Reason:    reason,
	}
}

// AccountClosed is emitted when an account is permanently closed.
type AccountClosed struct {
	domain.BaseEvent
}

// NewAccountClosed constructs an AccountClosed event.
func NewAccountClosed(eventID, aggregateID string, version int64) AccountClosed {
	return AccountClosed{
		BaseEvent: domain.NewBaseEvent(eventID, "account.closed", aggregateID, "Account", version),
	}
}

// OverdraftLimitChanged is emitted when SetOverdraftLimit accepts a new value.
type OverdraftLimitChanged struct {
	domain.BaseEvent
	Limit int64
}

// NewOverdraftLimitChanged constructs an OverdraftLimitChanged event.
func NewOverdraftLimitChanged(eventID, aggregateID string, version int64, limit int64) OverdraftLimitChanged {
	return OverdraftLimitChanged{
		BaseEvent: domain.NewBaseEvent(eventID, "account.overdraft_changed", aggregateID, "Account", version),
		Limit:     limit,
	}
}
