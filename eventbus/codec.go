// Package eventbus defines the event-driven contract built on top of
// watermill. Core exposes a Publisher/Subscriber abstraction that takes any
// watermill driver (Kafka, NATS, Redis, etc.), plus outbox/inbox interfaces
// for transactional messaging.
package eventbus

import (
	"context"

	"github.com/ThreeDotsLabs/watermill/message"

	"github.com/slam0504/go-ddd-core/domain"
)

// Codec translates DomainEvents to/from watermill messages. Codec is the
// single point where wire format (JSON, Protobuf, Avro...) is decided.
type Codec interface {
	// Marshal encodes a DomainEvent into a transport message. The returned
	// message should carry metadata that allows Unmarshal to reconstruct
	// the event type, such as an "event_name" header.
	Marshal(ctx context.Context, event domain.DomainEvent) (*message.Message, error)

	// Unmarshal decodes a transport message into a DomainEvent. It returns
	// the decoded event and the event name so the router can dispatch to
	// the correct typed handler.
	Unmarshal(ctx context.Context, msg *message.Message) (event domain.DomainEvent, name string, err error)
}

// Header keys used by the default marshalling conventions. Adapters and
// codecs should honour these where possible to keep tooling interoperable.
const (
	HeaderEventID       = "event_id"
	HeaderEventName     = "event_name"
	HeaderAggregateID   = "aggregate_id"
	HeaderAggregateType = "aggregate_type"
	HeaderOccurredAt    = "occurred_at"
	HeaderEventVersion  = "event_version"
	HeaderTraceID       = "trace_id"
	HeaderCausationID   = "causation_id"
	HeaderCorrelationID = "correlation_id"
)
