package filewatcher_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/yarcat/filewatcher-go"
	"github.com/yarcat/pubsub-go"
)

func Example() {
	dir, err := os.MkdirTemp("", "example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	notifier, err := filewatcher.NewFSNotify()
	if err != nil {
		log.Fatal(err)
	}
	watcher := filewatcher.New(notifier, pubsub.NewInprocess())

	dstFile := filepath.Join(dir, "fsnotify.txt")

	go func() { // Test scenario.
		sleep := func() { time.Sleep(1 * time.Second) }

		sleep() // No events.
		if err := os.WriteFile(dstFile, []byte("hello"), 0666); err != nil {
			log.Fatal(err)
		}
		sleep() // 2 events: CREATE + WRITE
		if err := os.Remove(dstFile); err != nil {
			log.Fatal(err)
		}
		sleep() // 1 event: REMOVE
		watcher.Close()
	}()

	it := watcher.MustWatch(dstFile)
	defer it.Close()
	for it.Next(context.Background()) {
		name := filepath.Base(it.Text())
		fmt.Printf("NAME: %q; ERROR: %v\n", name, it.Err())
		if errors.Is(it.Err(), context.DeadlineExceeded) {
			break
		} else if it.Err() != nil {
			continue
		}
		if b, err := os.ReadFile(it.Text()); os.IsNotExist(err) {
			fmt.Println("NO FILE")
		} else if err != nil {
			fmt.Println("OTHER ERROR")
		} else {
			fmt.Printf("CONTENT: %q\n", b)
		}
	}
	// Output:
	// NAME: "fsnotify.txt"; ERROR: <nil>
	// CONTENT: "hello"
	// NAME: "fsnotify.txt"; ERROR: <nil>
	// CONTENT: "hello"
	// NAME: "fsnotify.txt"; ERROR: <nil>
	// NO FILE
}
