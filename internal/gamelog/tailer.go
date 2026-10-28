package gamelog

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// Tailer читает application_*.log текущей сессии: новую сессию — с начала,
// дальше — только дописанные полные строки.
type Tailer struct {
	logsDir string
	onEvent func(Event)
	session string
	offsets map[string]int64
}

func NewTailer(logsDir string, onEvent func(Event)) *Tailer {
	return &Tailer{logsDir: logsDir, onEvent: onEvent, offsets: map[string]int64{}}
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
	}
	files, _ := filepath.Glob(filepath.Join(session, "*application_*.log"))
	sort.Strings(files)
	for _, f := range files {
		t.readFile(f)
	}
	return nil
}

func (t *Tailer) readFile(path string) {
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
		off = 0 // файл пересоздан
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
		if e, ok := ParseLine(string(bytes.TrimRight(line, "\r"))); ok {
			t.onEvent(e)
		}
	}
}
