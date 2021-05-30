package filewatcher

import (
	"errors"
	"log"

	"github.com/yarcat/pubsub-go"
)

// WatchCloser allows to inject actual watching implementations.
type WatchCloser interface {
	Watch(string, Publisher) error
	Close() error
}

// Publisher broadcasts data.
type Publisher struct{ pub pubsub.Publisher }

// Publish broadcasts the string.
func (pub Publisher) Publish(s string) error { return pub.pub.Publish(s) }

var ErrClosed = errors.New("file watcher is closed")

// FileWatcher listens to file changes. Close method should be called to free
// allocated resources.
type FileWatcher struct {
	topics  topics
	be      pubsub.Backend
	watcher WatchCloser
}

// New returns new file watcher ready to observe file changes. Close method
// should be called when this instance isn't needed anymore to free resources.
func New(watcher WatchCloser, be pubsub.Backend) FileWatcher {
	return FileWatcher{
		topics:  newTopics(),
		be:      be,
		watcher: watcher,
	}
}

// Close stops the watcher and frees allocated resources.
func (fw FileWatcher) Close() error {
	topics, err := fw.topics.Close()
	if err != nil {
		return nil
	}
	for _, t := range topics {
		// Topic.Close errors are ignored, but at least we should log them.
		if err := t.Close(); err != nil {
			log.Printf("FileWatcher.Close() = %v, want = nil", err)
		}
	}
	return fw.watcher.Close()
}

// Watch starts watching changes for the given file.
func (fw FileWatcher) Watch(name string) (*Iterator, error) {
	// TODO: Normalize name.

	var t pubsub.Topic

	if err := fw.topics.Run(func(topics nameTopicMap) error {
		var ok bool
		t, ok = topics[name]
		if !ok {
			t = pubsub.CreateTopic(fw.be)
			topics[name] = t
			pub := Publisher{pubsub.NewPublisher(t)}
			return fw.watcher.Watch(name, pub)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	sub, err := pubsub.Subscribe(t)
	if err != nil {
		return nil, err
	}
	return &Iterator{sub: sub}, nil
}

// MustWatch is a version of Watch which panics if there was an error.
func (fw FileWatcher) MustWatch(name string) *Iterator {
	it, err := fw.Watch(name)
	if err != nil {
		panic(err)
	}
	return it
}

type (
	nameTopicMap map[string]pubsub.Topic
	topics       struct {
		reg chan nameTopicMap
	}
)

func newTopics() topics {
	reg := make(chan nameTopicMap, 1)
	reg <- make(nameTopicMap)
	return topics{reg: reg}
}

func (t topics) Run(f func(nameTopicMap) error) error {
	reg, ok := <-t.reg
	if !ok {
		return ErrClosed
	}
	defer func() { t.reg <- reg }()
	return f(reg)
}

func (t topics) Close() (nameTopicMap, error) {
	reg, ok := <-t.reg
	if !ok {
		return nil, ErrClosed
	}
	close(t.reg)
	return reg, nil
}
