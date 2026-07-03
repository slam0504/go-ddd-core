package inboxtest_test

import (
	"context"
	"sync"
	"testing"

	"github.com/slam0504/go-ddd-core/eventbus"
	"github.com/slam0504/go-ddd-core/eventbus/inboxtest"
	"github.com/slam0504/go-ddd-core/pkg/errorsx"
)

// referenceInbox is the minimal in-memory conformer used to author and
// red-prove the suite. It is test-only — production in-memory inboxes live
// in go-ddd-adapters.
type referenceInbox struct {
	mu   sync.Mutex
	seen map[eventbus.InboxKey]struct{}
}

func newReference() *referenceInbox {
	return &referenceInbox{seen: make(map[eventbus.InboxKey]struct{})}
}

func validate(key eventbus.InboxKey) error {
	if key.Consumer == "" || key.EventID == "" {
		return errorsx.New(errorsx.CodeInvalidArgument, "inboxtest: empty consumer or event id")
	}
	return nil
}

func (r *referenceInbox) Seen(ctx context.Context, key eventbus.InboxKey) (bool, error) {
	if err := validate(key); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.seen[key]
	return ok, nil
}

func (r *referenceInbox) Record(ctx context.Context, key eventbus.InboxKey) error {
	if err := validate(key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.seen[key]; ok {
		return eventbus.ErrAlreadyRecorded
	}
	r.seen[key] = struct{}{}
	return nil
}

func TestRunContract_Reference(t *testing.T) {
	inboxtest.RunContract(t, func(t *testing.T) eventbus.Inbox {
		return newReference()
	})
}
