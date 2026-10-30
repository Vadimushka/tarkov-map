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
	stop chan struct{}
	done chan struct{}
}

// retryEvery — как часто проверять папку: игра создаёт Screenshots только при первом
// скрине, а игрок может её удалить или переименовать и создать заново.
var retryEvery = 2 * time.Second

// NewWatcher следит за dir; если папки нет (ещё нет или её удалили), начнёт следить,
// когда она появится. Так же подхватывается новая папка на месте переименованной.
func NewWatcher(dir string, onShot func(name string, at time.Time)) (*Watcher, error) {
	d, err := watchDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	w := &Watcher{stop: make(chan struct{}), done: make(chan struct{})}
	go w.run(dir, d, onShot)
	return w, nil
}

func (w *Watcher) run(dir string, d *dirWatch, onShot func(name string, at time.Time)) {
	defer close(w.done)
	defer func() { d.close() }()
	t := time.NewTicker(retryEvery)
	defer t.Stop()
	for {
		var events <-chan fsnotify.Event
		var errs <-chan error
		if d != nil {
			events, errs = d.fw.Events, d.fw.Errors
		}
		select {
		case <-t.C:
			if d.gone(dir) {
				d.close()
				d = nil
			}
			if d == nil {
				if nd, err := watchDir(dir); err == nil {
					d = nd
					// игра создаёт папку вместе с первым скрином — Create для него уже не придёт
					reportExisting(dir, onShot)
				}
			}
		case ev, ok := <-events:
			if !ok {
				d = nil
				continue
			}
			if ev.Has(fsnotify.Create) && strings.EqualFold(filepath.Ext(ev.Name), ".png") {
				onShot(filepath.Base(ev.Name), time.Now())
			}
			// удалили или переименовали саму папку (так сообщает Linux) — ждём новую
			if ev.Has(fsnotify.Remove|fsnotify.Rename) && samePath(ev.Name, dir) {
				d.close()
				d = nil
			}
		case _, ok := <-errs:
			if !ok {
				d = nil
			}
		case <-w.stop:
			return
		}
	}
}

func (w *Watcher) Close() {
	close(w.stop)
	<-w.done
}

// dirWatch — слежение за одной папкой; id — какая это папка, чтобы заметить подмену.
type dirWatch struct {
	fw *fsnotify.Watcher
	id os.FileInfo
}

// watchDir начинает следить за dir; папки нет — ошибка с fs.ErrNotExist.
func watchDir(dir string) (*dirWatch, error) {
	id, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	os.SameFile(id, id) // Windows читает номер папки лениво — прочитать, пока папка та же
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fw.Add(dir); err != nil {
		fw.Close()
		return nil, err
	}
	return &dirWatch{fw: fw, id: id}, nil
}

// gone — по пути dir папки больше нет или там уже другая: прежнюю переименовали, а
// Windows об этом не сообщает и шлёт её события с прежним путём. Прочие ошибки
// (нет доступа, сбой диска) слежение не снимают.
func (d *dirWatch) gone(dir string) bool {
	if d == nil {
		return false
	}
	cur, err := os.Stat(dir)
	if err != nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	return !os.SameFile(cur, d.id)
}

func (d *dirWatch) close() {
	if d != nil {
		d.fw.Close()
	}
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

// samePath — пути указывают на одну папку (регистр в Windows не важен).
func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
