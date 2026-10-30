package data

import (
	"errors"
	"fmt"
	"sync"
)

// Syncer держит в Store данные нужного режима так, чтобы сеть не задерживала карту:
// кэш любой давности загружается сразу, устаревший обновляется в фоне. Неудача сети
// запоминается до конца запуска (или до Refresh): на входе в игру режим в логах мигает
// Pve → Regular → Pve, и каждая попытка через «висящий» прокси стоила бы десятки секунд.
type Syncer struct {
	store    *Store
	onChange func() // данные в Store обновились в фоне

	mu     sync.Mutex
	mode   string          // режим данных, загруженных в Store
	failed map[string]bool // режимы, для которых сеть не ответила
	busy   map[string]bool // идёт фоновое обновление
	wg     sync.WaitGroup
}

func NewSyncer(s *Store, onChange func()) *Syncer {
	return &Syncer{store: s, onChange: onChange, failed: map[string]bool{}, busy: map[string]bool{}}
}

// Use делает текущими данные режима. Нет кэша — скачивает (одна попытка за запуск);
// без кэша и сети — ErrNoData.
func (y *Syncer) Use(mode string) error {
	y.mu.Lock()
	defer y.mu.Unlock()
	if mode != y.mode {
		if err := y.store.LoadCached(mode); err != nil {
			if y.failed[mode] {
				return err
			}
			// файлы есть, но не разбираются (сбой записи) — перекачать все, даже «свежие»
			if ferr := y.store.Fetch(mode, !errors.Is(err, ErrNoData)); ferr != nil {
				y.failed[mode] = true
				return fmt.Errorf("%w (%v)", ErrNoData, ferr)
			}
			if err := y.store.LoadCached(mode); err != nil {
				return err
			}
		}
		y.mode = mode
	}
	if y.store.Stale(mode) && !y.failed[mode] && !y.busy[mode] {
		y.busy[mode] = true
		y.wg.Add(1)
		go y.refresh(mode)
	}
	return nil
}

func (y *Syncer) refresh(mode string) {
	defer y.wg.Done()
	err := y.store.Fetch(mode, false) // долго — без замка
	y.mu.Lock()
	delete(y.busy, mode)
	changed := false
	if err != nil {
		y.failed[mode] = true
	} else if y.mode == mode { // режим мог смениться, пока качали
		changed = y.store.LoadCached(mode) == nil
	}
	y.mu.Unlock()
	if changed && y.onChange != nil {
		y.onChange()
	}
}

// Refresh — кнопка «Обновить данные»: скачать сейчас, забыв прежние неудачи.
// При ошибке сети данные остаются на кэше, ошибка возвращается.
func (y *Syncer) Refresh(mode string) error {
	y.mu.Lock()
	y.failed = map[string]bool{}
	y.mu.Unlock()
	err := y.store.Fetch(mode, true)
	y.mu.Lock()
	defer y.mu.Unlock()
	if err != nil {
		y.failed[mode] = true
		return fmt.Errorf("сеть не ответила: %v", err)
	}
	if err := y.store.LoadCached(mode); err != nil {
		return err
	}
	y.mode = mode
	return nil
}

// Mode — режим данных, загруженных в Store ("" — ещё никаких).
func (y *Syncer) Mode() string {
	y.mu.Lock()
	defer y.mu.Unlock()
	return y.mode
}

// Wait ждёт фоновые обновления (для тестов и выхода).
func (y *Syncer) Wait() { y.wg.Wait() }
