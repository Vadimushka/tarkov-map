package data

import (
	"errors"
	"testing"
	"time"
)

// Сеть «висит» (прокси без выхода наружу), кэш устарел: карта нужна сразу, обновление — в фоне.
func TestSyncerStaleCacheServedAtOnce(t *testing.T) {
	_, fs, c, dir := loaded(t)
	c.t = c.t.Add(25 * time.Hour)
	release := fs.hangUntilCleanup(t)
	changed := make(chan struct{}, 1)
	s := newStore(t, fs, c, dir) // новый запуск приложения
	y := NewSyncer(s, func() { changed <- struct{}{} })

	start := time.Now()
	if err := y.Use("pve"); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Use ждал сеть %v при живом кэше", d)
	}
	if _, ok := s.MapByScene("maps/shopping_mall.bundle"); !ok {
		t.Error("данные кэша не загружены")
	}
	release()
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Error("после фонового обновления не было onChange")
	}
	y.Wait()
	if s.Stale("pve") {
		t.Error("кэш не обновился в фоне")
	}
}

func TestSyncerNoCacheFetches(t *testing.T) {
	fs := newServer(t)
	s := newStore(t, fs, &clock{t: time.Now()}, t.TempDir())
	y := NewSyncer(s, nil)
	if err := y.Use("pve"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.MapByScene("maps/shopping_mall.bundle"); !ok {
		t.Error("данные не скачаны")
	}
	y.Wait()
}

// На входе в игру режим в логах мигает Pve → Regular → Pve: неудача сети не должна повторяться.
func TestSyncerNetworkFailureNotRetried(t *testing.T) {
	fs := newServer(t)
	fs.fail.Store(true)
	s := newStore(t, fs, &clock{t: time.Now()}, t.TempDir())
	y := NewSyncer(s, nil)
	if err := y.Use("pve"); !errors.Is(err, ErrNoData) {
		t.Fatalf("без кэша и сети: %v, want ErrNoData", err)
	}
	hits := fs.hits.Load()
	if err := y.Use("pve"); !errors.Is(err, ErrNoData) {
		t.Fatalf("повтор: %v", err)
	}
	if fs.hits.Load() != hits {
		t.Error("после неудачи сеть дёргается снова")
	}
	fs.fail.Store(false)
	if err := y.Refresh("pve"); err != nil {
		t.Fatalf("Refresh после восстановления сети: %v", err)
	}
	if _, ok := s.MapByScene("maps/shopping_mall.bundle"); !ok {
		t.Error("Refresh не загрузил данные")
	}
	y.Wait()
}

func TestSyncerStaleRefreshFailureNotRetried(t *testing.T) {
	_, fs, c, dir := loaded(t)
	c.t = c.t.Add(25 * time.Hour)
	fs.fail.Store(true)
	y := NewSyncer(newStore(t, fs, c, dir), nil)
	if err := y.Use("pve"); err != nil {
		t.Fatal(err)
	}
	y.Wait()
	hits := fs.hits.Load()
	if err := y.Use("pve"); err != nil {
		t.Fatal(err)
	}
	y.Wait()
	if fs.hits.Load() != hits {
		t.Error("неудачное фоновое обновление повторяется на каждом Use")
	}
}

func TestSyncerRefreshKeepsCacheOnFailure(t *testing.T) {
	_, fs, c, dir := loaded(t)
	s := newStore(t, fs, c, dir)
	y := NewSyncer(s, nil)
	if err := y.Use("pve"); err != nil {
		t.Fatal(err)
	}
	fs.fail.Store(true)
	if err := y.Refresh("pve"); err == nil {
		t.Error("Refresh без сети вернул nil")
	}
	if _, ok := s.MapByScene("maps/shopping_mall.bundle"); !ok {
		t.Error("после неудачного Refresh данные пропали")
	}
	y.Wait()
}
