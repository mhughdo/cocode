package eventbus

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
	"github.com/hughdo/cocode/services/cocoded/internal/eventlog"
)

const defaultSubscriberBuffer = 64

type Bus struct {
	store *eventlog.Store

	mu          sync.Mutex
	nextID      int
	subscribers map[string]map[int]chan dbgen.Event
}

func New(store *eventlog.Store) (*Bus, error) {
	if store == nil {
		return nil, errors.New("event store is required")
	}
	return &Bus{store: store}, nil
}

func (b *Bus) Append(ctx context.Context, params eventlog.AppendParams) (dbgen.Event, error) {
	if b == nil || b.store == nil {
		return dbgen.Event{}, errors.New("event bus is required")
	}
	event, err := b.store.Append(ctx, params)
	if err != nil {
		return dbgen.Event{}, err
	}
	b.publish(event)
	return event, nil
}

func (b *Bus) ListByReviewSession(ctx context.Context, reviewSessionID string) ([]dbgen.Event, error) {
	if b == nil || b.store == nil {
		return nil, errors.New("event bus is required")
	}
	return b.store.ListByReviewSession(ctx, reviewSessionID)
}

func (b *Bus) Subscribe(reviewSessionID string) (<-chan dbgen.Event, func(), error) {
	if b == nil || b.store == nil {
		return nil, nil, errors.New("event bus is required")
	}
	reviewSessionID = strings.TrimSpace(reviewSessionID)
	if reviewSessionID == "" {
		return nil, nil, errors.New("review session id is required")
	}
	ch := make(chan dbgen.Event, defaultSubscriberBuffer)

	b.mu.Lock()
	if b.subscribers == nil {
		b.subscribers = map[string]map[int]chan dbgen.Event{}
	}
	if b.subscribers[reviewSessionID] == nil {
		b.subscribers[reviewSessionID] = map[int]chan dbgen.Event{}
	}
	id := b.nextID
	b.nextID++
	b.subscribers[reviewSessionID][id] = ch
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		sessionSubscribers := b.subscribers[reviewSessionID]
		if sessionSubscribers == nil {
			return
		}
		if current := sessionSubscribers[id]; current != nil {
			delete(sessionSubscribers, id)
			close(current)
		}
		if len(sessionSubscribers) == 0 {
			delete(b.subscribers, reviewSessionID)
		}
	}
	return ch, unsubscribe, nil
}

func (b *Bus) publish(event dbgen.Event) {
	if b == nil {
		return
	}
	if !event.ReviewSessionID.Valid {
		return
	}
	b.mu.Lock()
	subscribers := b.subscribers[event.ReviewSessionID.String]
	channels := make([]chan dbgen.Event, 0, len(subscribers))
	for _, ch := range subscribers {
		channels = append(channels, ch)
	}
	b.mu.Unlock()

	for _, ch := range channels {
		select {
		case ch <- event:
		default:
		}
	}
}
