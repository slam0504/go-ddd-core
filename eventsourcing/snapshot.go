package eventsourcing

import (
	"context"
	"time"
)

// Snapshot captures aggregate state at a given version so replay can start
// from a recent checkpoint instead of the beginning of the stream.
type Snapshot struct {
	Stream  StreamID
	Version int64
	State   []byte
	TakenAt time.Time
}

// SnapshotStore persists and retrieves aggregate snapshots. Encoding of
// State is left to the caller (JSON, Protobuf, CBOR...).
type SnapshotStore interface {
	Save(ctx context.Context, snap Snapshot) error
	Latest(ctx context.Context, stream StreamID) (snap Snapshot, found bool, err error)
}

// SnapshotPolicy decides whether to take a snapshot after appending events.
type SnapshotPolicy interface {
	ShouldSnapshot(stream StreamID, version int64, appended int) bool
}

// EveryN returns a SnapshotPolicy that snapshots every n versions.
func EveryN(n int64) SnapshotPolicy {
	return everyN{n: n}
}

type everyN struct{ n int64 }

func (p everyN) ShouldSnapshot(_ StreamID, version int64, _ int) bool {
	if p.n <= 0 {
		return false
	}
	return version%p.n == 0
}
