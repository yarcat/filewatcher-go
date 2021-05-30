package filewatcher

import (
	"log"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

type namePub struct {
	Name string
	Pub  Publisher
}

type fsnotifyCommands struct {
	Done  <-chan struct{}
	Watch <-chan namePub
}

// FSNotify implements fsnotify watcher.
type FSNotify struct {
	done     chan struct{}
	finished chan error
	watch    chan namePub
}

func NewFSNotify() (FSNotify, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return FSNotify{}, err
	}
	fsn := FSNotify{
		done:     make(chan struct{}),
		finished: make(chan error),
		watch:    make(chan namePub),
	}
	go func() {
		defer close(fsn.finished)
		fsn.finished <- fsnotifyLoop(
			fsnotifyCommands{Done: fsn.done, Watch: fsn.watch},
			fsw,
		)
	}()
	return fsn, nil
}

func (fsn FSNotify) Watch(name string, pub Publisher) error {
	select {
	case <-fsn.done:
		return ErrClosed
	case fsn.watch <- namePub{Name: name, Pub: pub}:
		return nil
	}
}

// Close stop the watcher, releases allocated resources and waits for underlying
// go routines to complete.
func (fsn FSNotify) Close() error {
	select {
	case <-fsn.done:
		return ErrClosed
	default:
	}
	close(fsn.done)           // Signal to finish.
	err, ok := <-fsn.finished // Wait for the termination.
	if !ok {
		err = ErrClosed
	}
	return err
}

func fsnotifyLoop(cmds fsnotifyCommands, fsw *fsnotify.Watcher) (err error) {
	log.Println("fsnotifyLoop: start")
	router := make(map[string]Publisher)
	for {
		select {
		case <-cmds.Done:
			log.Println("fsnotifyLoop: closing")
			cmds.Done = nil // Done won't trigger anymore.
			go fsw.Close()
		case watch := <-cmds.Watch:
			dir := filepath.Dir(watch.Name)
			if err := fsw.Add(dir); err != nil {
				log.Printf("fsnotifyLoop: watch %v: %v", watch.Name, err)
			} else {
				log.Println("fsnotifyLoop: watch", watch.Name)
			}
			router[watch.Name] = watch.Pub
		case ev, ok := <-fsw.Events:
			if !ok {
				log.Println("fsnotifyLoop: return from events")
				return nil
			}
			log.Printf("fsnotifyLoop: %v", ev)
			if pub, ok := router[ev.Name]; ok {
				pub.Publish(ev.Name)
			} else {
				log.Printf("fsnotifyLoop: cannot find publisher for %v", ev.Name)
				log.Println(router)
			}
		case err, ok := <-fsw.Errors:
			if !ok {
				log.Println("fsnotifyLoop: return from errors")
				return err
			}
			log.Printf("fsnotifyLoop got err = %v, want = nil", err)
		}
	}
}
