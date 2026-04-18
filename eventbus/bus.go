package eventbus

import (
	"context"

	"github.com/ThreeDotsLabs/watermill/message"

	"github.com/slam0504/go-ddd-core/domain"
)

// Publisher publishes domain events to a topic. Implementations typically
// compose a watermill message.Publisher (Kafka, NATS, etc.) with a Codec.
type Publisher interface {
	Publish(ctx context.Context, topic string, events ...domain.DomainEvent) error
	Close() error
}

// Subscriber delivers decoded domain events from a topic. The returned
// channel closes when the context is cancelled or the subscriber is closed.
type Subscriber interface {
	Subscribe(ctx context.Context, topic string) (<-chan Envelope, error)
	Close() error
}

// Envelope couples a decoded DomainEvent with its raw transport message so
// consumers can access metadata and control ack/nack.
type Envelope struct {
	Event domain.DomainEvent
	Name  string
	Raw   *message.Message
}

// Ack acknowledges successful processing of the underlying message.
func (e Envelope) Ack() { e.Raw.Ack() }

// Nack signals processing failure so the broker can redeliver.
func (e Envelope) Nack() { e.Raw.Nack() }
