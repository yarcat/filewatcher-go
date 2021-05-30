package filewatcher_test

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/yarcat/filewatcher-go"
	"github.com/yarcat/pubsub-go"
)

func TestFileWatcher(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  testContext
	}{{
		name: "close must close watcher",
		ctx: newContext(
			withActions(closeWatcher()),
			withChecks(isClosed),
		),
	}, {
		name: "watch creates iterator",
		ctx: newContext(
			withActions(
				mustWatch("foo.bar"),
			),
			withChecks(
				iterators{"foo.bar"}.len().eq(1),
			),
		),
	}, {
		name: "close closes iterator",
		ctx: newContext(
			withActions(
				mustWatch("foo.bar"),
				closeWatcher(),
				iterators{"foo.bar"}.expose(),
			),
			withChecks(
				exposedIterators{"foo.bar"}.len().eq(1),
			),
		),
	}, {
		name: "watch creates iterators",
		ctx: newContext(
			withActions(
				mustWatch("foo.bar"),
				mustWatch("bar.foo"),
				mustWatch("foo.bar"),
			),
			withChecks(
				iterators{"foo.bar"}.len().eq(2),
				iterators{"bar.foo"}.len().eq(1),
			),
		),
	}, {
		name: "close closes iterators",
		ctx: newContext(
			withActions(
				mustWatch("foo.bar"),
				mustWatch("bar.foo"),
				mustWatch("foo.bar"),
				closeWatcher(),
				iterators{"foo.bar"}.expose(),
				iterators{"bar.foo"}.expose(),
			),
			withChecks(
				exposedIterators{"foo.bar"}.len().eq(2),
				exposedIterators{"bar.foo"}.len().eq(1),
			),
		),
	}, {
		name: "must publish messages",
		ctx: newContext(
			withActions(
				mustWatch("foo.bar"),
				mustWatch("bar.foo"),
				mustWatch("foo.bar"),
				mustPublish("foo.bar", "change1"),
				mustPublish("foo.bar", "change2"),
				stashMessages("foo.bar"),
				stashMessages("bar.foo"),
			),
			withChecks(
				stashedMessages("foo.bar").eq(
					// Sent 2 messages with two iterators.
					"change1", "change2", "change1", "change2",
				),
				stashedMessages("bar.foo").isEmpty(),
				stashedErrors("foo.bar").eq(
					context.DeadlineExceeded.Error(),
					context.DeadlineExceeded.Error(),
				),
				stashedErrors("bar.foo").eq(
					context.DeadlineExceeded.Error(),
				),
			),
		),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			defer tc.ctx.Close()

			fw := filewatcher.New(tc.ctx.Watcher(), pubsub.NewInprocess())
			defer fw.Close()

			tc.ctx.ExecuteActions(fw)
			tc.ctx.ExecuteChecks(t)
		})
	}
}

func closeWatcher() actionFunc {
	return func(ctx *testContext, fw filewatcher.FileWatcher) {
		log.Println("TEST: CLOSING WATCHER")
		fw.Close()
	}
}

func stashMessages(name string) actionFunc {
	return func(tc *testContext, fw filewatcher.FileWatcher) {
		log.Println("TEST: STASH NOTIFICATIONS FOR", name)
		tc.stash(name)
	}
}

func mustPublish(name, message string) actionFunc {
	return func(tc *testContext, fw filewatcher.FileWatcher) {
		log.Println("TEST: MUST PUBLISH FOR", name, "MESSAGE", message)
		pubs, ok := tc.publishers[name]
		if !ok {
			return
		}
		for _, p := range pubs {
			if err := p.Publish(message); err != nil {
				panic(err)
			}
		}

	}
}

func isClosed(t *testing.T, ctx *testContext) {
	if !ctx.closed {
		t.Error("WatchCloser.Close was not called")
	}
}

type (
	iteratorRegistry map[string][]*filewatcher.Iterator
	iterators        struct{ name string }
	exposedIterators struct{ name string }
	itCmp            struct {
		name     string
		regName  string
		registry func(*testContext) iteratorRegistry
	}
	stash struct {
		name      string
		stashName string
		stash     func(*testContext) []string
	}
)

func stashedMessages(name string) *stash {
	return &stash{
		name:      name,
		stashName: "messageStash",
		stash:     func(tc *testContext) []string { return tc.messageStash[name] },
	}
}

func stashedErrors(name string) *stash {
	return &stash{
		name:      name,
		stashName: "errorStash",
		stash:     func(tc *testContext) []string { return tc.errorStash[name] },
	}
}

func (s *stash) isEmpty() checkFunc { return s.eq() }

func (s *stash) eq(want ...string) checkFunc {
	return func(t *testing.T, tc *testContext) {
		got := s.stash(tc)
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("got unexpected diff with %s[%s] (-want, +got): %v",
				s.stashName, s.name, diff)
		}
	}
}

func (it iterators) len() itCmp {
	return itCmp{
		name:     it.name,
		regName:  "iterators",
		registry: func(tc *testContext) iteratorRegistry { return tc.iterators },
	}
}

func (it exposedIterators) len() itCmp {
	return itCmp{
		name:     it.name,
		regName:  "exposed",
		registry: func(tc *testContext) iteratorRegistry { return tc.exposed },
	}
}

func (it iterators) expose() actionFunc {
	return func(tc *testContext, fw filewatcher.FileWatcher) {
		log.Println("TEST: EXPOSING ITERATORS FOR", it.name)
		tc.collect(it.name, -1, nil)
	}
}

func (it itCmp) eq(want int) checkFunc {
	return func(t *testing.T, tc *testContext) {
		if got := len(tc.iterators[it.name]); got != want {
			t.Errorf("len(%s[%s]) = %v, want = %v",
				it.regName, it.name, got, want)
		}
	}
}

func mustWatch(name string) actionFunc {
	return func(tc *testContext, fw filewatcher.FileWatcher) {
		log.Println("TEST: WATCHING", name)
		tc.addIterator(name, fw.MustWatch(name))
	}
}

type (
	checkFunc     func(*testing.T, *testContext)
	actionFunc    func(*testContext, filewatcher.FileWatcher)
	contextOption func(*testContext)
	testContext   struct {
		checks       []checkFunc
		actions      []actionFunc
		closed       bool
		iterators    iteratorRegistry
		exposed      iteratorRegistry
		publishers   map[string][]filewatcher.Publisher
		messageStash map[string][]string
		errorStash   map[string][]string
	}
)

func newContext(opts ...contextOption) testContext {
	ctx := testContext{
		iterators:    make(iteratorRegistry),
		exposed:      make(iteratorRegistry),
		publishers:   make(map[string][]filewatcher.Publisher),
		messageStash: make(map[string][]string),
		errorStash:   make(map[string][]string),
	}
	for _, opt := range opts {
		opt(&ctx)
	}
	return ctx
}

func withChecks(checks ...checkFunc) contextOption {
	return func(tc *testContext) { tc.checks = checks }

}

func withActions(actions ...actionFunc) contextOption {
	return func(tc *testContext) { tc.actions = actions }
}

func (ctx *testContext) addIterator(name string, it *filewatcher.Iterator) {
	ctx.iterators[name] = append(ctx.iterators[name], it)
}

func (ctx *testContext) ExecuteChecks(t *testing.T) {
	for _, check := range ctx.checks {
		check(t, ctx)
	}
}

func (ctx *testContext) ExecuteActions(fw filewatcher.FileWatcher) {
	for _, action := range ctx.actions {
		action(ctx, fw)
	}
}

func (ctx *testContext) Close() {}

func (ctx *testContext) stash(name string) {
	ctx.collect(name, -1, func(s string, e error) {
		if s != "" {
			ctx.messageStash[name] = append(ctx.messageStash[name], s)
		}
		if e != nil {
			ctx.errorStash[name] = append(ctx.errorStash[name], e.Error())
		}
	})
}

func (tc *testContext) collect(name string, cnt int, sink func(string, error)) {
	if sink == nil {
		sink = func(s string, e error) {}
	}
	for _, fwit := range tc.iterators[name] {
		c := collector{Count: cnt, Sink: sink, Iter: fwit}
		if exposed := c.Collect(); exposed {
			tc.exposed[name] = append(tc.exposed[name], fwit)
		}
	}
}

func (ctx *testContext) Watcher() *testWatcher {
	return &testWatcher{ctx: ctx}
}

type testWatcher struct{ ctx *testContext }

func (tw *testWatcher) Watch(s string, pub filewatcher.Publisher) error {
	tw.ctx.publishers[s] = append(tw.ctx.publishers[s], pub)
	return nil
}

func (tw *testWatcher) Close() error {
	tw.ctx.closed = true
	return nil
}

type collector struct {
	Count int
	Sink  func(string, error)
	Iter  *filewatcher.Iterator
}

var defaultTimeout = 100 * time.Millisecond

func (c collector) Collect() (exposed bool) {
	// Setting small timeouts to ensure we don't get blocked in the reader if
	// there is no data sent. Setting a smaller timeout before every call
	// allows other go routines to operate without flakiness.
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if c.Count < 0 {
		// Collect all messages until closed or cancelled.
		for c.Iter.Next(ctx) {
			c.Sink(c.Iter.Text(), c.Iter.Err())
			if ctx.Err() != nil && c.Iter.Err() == ctx.Err() { // Cancelled.
				return false
			}
		}
		return true
	}
	// Collect c.Count messages until collected, closed or cancelled.
	for i := 0; i < c.Count && c.Iter.Next(ctx); i++ {
		c.Sink(c.Iter.Text(), c.Iter.Err())
		if ctx.Err() != nil && c.Iter.Err() == ctx.Err() { // Cancelled.
			return false
		}
	}
	return true
}
