package eventsourcing

import (
	"context"

	"github.com/slam0504/go-ddd-core/domain"
)

// Projector builds a read model from events. Projectors are idempotent —
// replaying the same event twice yields the same read-model state.
type Projector interface {
	// Name uniquely identifies this projector so its checkpoint can be
	// tracked independently of others.
	Name() string
	// Project applies an event to the read model.
	Project(ctx context.Context, event PersistedEvent) error
}

// ProjectorFunc adapts a function to the Projector interface. Name is
// captured at construction time.
type ProjectorFunc struct {
	ProjectorName string
	ProjectFn     func(ctx context.Context, event PersistedEvent) error
}

func (p ProjectorFunc) Name() string { return p.ProjectorName }

func (p ProjectorFunc) Project(ctx context.Context, event PersistedEvent) error {
	return p.ProjectFn(ctx, event)
}

// CheckpointStore tracks per-projector read position so recovery can resume
// instead of replaying from zero.
type CheckpointStore interface {
	Load(ctx context.Context, projector string) (position int64, err error)
	Save(ctx context.Context, projector string, position int64) error
}

// Reactor is a projector-like component that produces side effects (sending
// notifications, issuing further commands, etc.). Kept as a distinct type so
// failure handling and retry semantics can differ from read-model projection.
type Reactor interface {
	Name() string
	React(ctx context.Context, event domain.DomainEvent) error
}
