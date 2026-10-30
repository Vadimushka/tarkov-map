package shots

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher сообщает о новых *.png в папке. Сам файл не открывается:
// его может удалить другой процесс сразу после создания (скрипт TM).
type Watcher struct {
	fw   *fsnotify.Watcher
	stop chan struct{}
	done chan struct{}
}

// retryEvery — как часто проверять, не появилась ли папка: игра создаёт Screenshots
// только при первом скрине.
var retryEvery = 2 * time.Second

// NewWatcher следит за dir; если папки ещё нет, начнёт следить, когда она появится.
func NewWatcher(dir string, onShot func(name string, at time.Time)) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{fw: fw, stop: make(chan struct{}), done: make(chan struct{})}
	waiting := false
	if err := fw.Add(dir); err != nil {
		if _, serr := os.Stat(dir); !errors.Is(serr, fs.ErrNotExist) {
			fw.Close()
			return nil, err
		}
		waiting = true
	}
	go func() {
		defer close(w.done)
		var retry <-chan time.Time
		if waiting {
			t := time.NewTicker(retryEvery)
			defer t.Stop()
			retry = t.C
		}
		for {
			select {
			case <-retry:
				if fw.Add(dir) == nil {
					retry = nil
					// игра создаёт папку вместе с первым скрином — Create для него уже не придёт
					reportExisting(dir, onShot)
				}
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
			case <-w.stop:
				return
			}
		}
	}()
	return w, nil
}

func (w *Watcher) Close() error {
	close(w.stop)
	err := w.fw.Close()
	<-w.done
	return err
}

// reportExisting сообщает о *.png, уже лежащих в папке; повторы отсеивает ядро по ключу скрина.
func reportExisting(dir string, onShot func(name string, at time.Time)) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".png") {
			continue
		}
		if info, err := e.Info(); err == nil {
			onShot(e.Name(), info.ModTime())
		}
	}
}
