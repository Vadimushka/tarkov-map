package gamelog

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
)

// LatestSession возвращает подпапку логов с самым свежим временем изменения.
func LatestSession(logsDir string) (string, error) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return "", err
	}
	var best string
	var bestMod int64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if m := info.ModTime().UnixNano(); best == "" || m > bestMod {
			best, bestMod = filepath.Join(logsDir, e.Name()), m
		}
	}
	if best == "" {
		return "", os.ErrNotExist
	}
	return best, nil
}

// Tailer читает application_*.log и push-notifications_*.log текущей сессии: новую
// сессию — с начала, дальше — только дописанные полные строки.
type Tailer struct {
	logsDir string
	onEvent func(Event)
	session string
	offsets map[string]int64
	quests  map[string]*questReader // по файлу уведомлений: блок может прийти за два опроса
}

func NewTailer(logsDir string, onEvent func(Event)) *Tailer {
	return &Tailer{logsDir: logsDir, onEvent: onEvent, offsets: map[string]int64{}, quests: map[string]*questReader{}}
}

// Poll вызывается раз в секунду.
func (t *Tailer) Poll() error {
	session, err := LatestSession(t.logsDir)
	if err != nil {
		return nil // игры ещё нет или папка не найдена — ждём
	}
	if session != t.session {
		t.session = session
		t.offsets = map[string]int64{}
		t.quests = map[string]*questReader{}
	}
	for _, f := range logFiles(session, "*application_*.log") {
		t.readFile(f, ParseLine)
	}
	for _, f := range logFiles(session, "*push-notifications_*.log") {
		r := t.quests[f]
		if r == nil {
			r = &questReader{}
			t.quests[f] = r
		}
		t.readFile(f, r.line)
	}
	return nil
}

func (t *Tailer) readFile(path string, parse func(string) (Event, bool)) {
	h, err := os.Open(path)
	if err != nil {
		return
	}
	defer h.Close()
	info, err := h.Stat()
	if err != nil {
		return
	}
	off := t.offsets[path]
	if info.Size() < off {
		off = 0 // файл пересоздан; недочитанный блок уведомления отбросит первая же строка лога
	}
	if info.Size() == off {
		return
	}
	if _, err := h.Seek(off, io.SeekStart); err != nil {
		return
	}
	buf, err := io.ReadAll(h)
	if err != nil {
		return
	}
	last := bytes.LastIndexByte(buf, '\n')
	if last < 0 {
		return // неполная строка — дочитаем позже
	}
	t.offsets[path] = off + int64(last) + 1
	for _, line := range bytes.Split(buf[:last], []byte{'\n'}) {
		if e, ok := parse(string(bytes.TrimRight(line, "\r"))); ok {
			t.onEvent(e)
		}
	}
}
