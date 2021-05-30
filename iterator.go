package filewatcher

import (
	"context"

	"github.com/yarcat/pubsub-go"
)

// Iterators iterates every time there is a file change.
type Iterator struct {
	sub    pubsub.Subscription
	s      string
	err    error
	closed bool
}

// Next blocks until an event happens. Returned value indicates whether this
// iterator is still valid after the recent event received.
func (it *Iterator) Next(ctx context.Context) bool {
	if it.closed {
		return false
	}
	b, err := it.sub.Receive(ctx)
	if err == pubsub.ErrTopicClosed {
		it.Close()
		return false
	}
	it.s, _ = b.(string)
	it.err = err
	return true
}

// Close stop the iterator and frees allocated resources.
func (it *Iterator) Close() error {
	if !it.closed {
		it.closed, it.s, it.err = true, "", it.sub.Disconnect()
	}
	return it.err
}

// Text returns string received by the last Next() call.
func (it *Iterator) Text() string { return it.s }

// Err returns an error received by the last Next() call. Note that stream
// termination error is ignored and Next() always returns false afterwards.
func (it *Iterator) Err() error { return it.err }
