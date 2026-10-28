package shots

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher сообщает о новых *.png в папке. Сам файл не открывается:
// его может удалить другой процесс сразу после создания (скрипт TM).
type Watcher struct {
	fw   *fsnotify.Watcher
	done chan struct{}
}

func NewWatcher(dir string, onShot func(name string, at time.Time)) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fw.Add(dir); err != nil {
		fw.Close()
		return nil, err
	}
	w := &Watcher{fw: fw, done: make(chan struct{})}
	go func() {
		defer close(w.done)
		for {
			select {
			case ev, ok := <-fw.Events:
				if !ok {
					return
				}
				if ev.Has(fsnotify.Create) && strings.EqualFold(filepath.Ext(ev.Name), ".png") {
					onShot(filepath.Base(ev.Name), time.Now())
				}
			case _, ok := <-fw.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return w, nil
}

func (w *Watcher) Close() error {
	err := w.fw.Close()
	<-w.done
	return err
}
