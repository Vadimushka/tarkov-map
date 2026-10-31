package gamelog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Тип сообщения о задании в push-notifications: торговец пишет при взятии, провале и сдаче.
var questStatus = map[int]string{10: "start", 11: "fail", 12: "done"}

// questReader собирает уведомления о заданиях из push-notifications_*.log: строка
// «Got notification | ChatMessageReceived», за ней JSON до «}» в начале строки (или до
// следующей строки лога). Блок может прийти за несколько опросов — состояние хранится.
type questReader struct {
	on  bool
	at  time.Time
	buf strings.Builder
}

func (r *questReader) line(s string) (Event, bool) {
	at, stamped := lineTime(s)
	if r.on && stamped { // JSON кончился без «}» — строка лога сама может начать новый блок
		e, ok := r.finish()
		r.start(s, at)
		return e, ok
	}
	if !r.on {
		r.start(s, at)
		return Event{}, false
	}
	r.buf.WriteString(s)
	r.buf.WriteByte('\n')
	if s == "}" {
		return r.finish()
	}
	return Event{}, false
}

func (r *questReader) start(s string, at time.Time) bool {
	if !strings.Contains(s, "Got notification | ChatMessageReceived") {
		return false
	}
	r.on, r.at = true, at
	r.buf.Reset()
	return true
}

func (r *questReader) finish() (Event, bool) {
	r.on = false
	var n struct {
		Message struct {
			Type       int    `json:"type"`
			TemplateID string `json:"templateId"` // «<id задания> description»
		} `json:"message"`
	}
	if json.Unmarshal([]byte(r.buf.String()), &n) != nil {
		return Event{}, false
	}
	status, ok := questStatus[n.Message.Type]
	id, _, _ := strings.Cut(n.Message.TemplateID, " ")
	if !ok || id == "" {
		return Event{}, false
	}
	return Event{Kind: EvQuest, At: r.at, QuestID: id, Status: status}, true
}

func lineTime(s string) (time.Time, bool) {
	if len(s) < len(timeLayout) {
		return time.Time{}, false
	}
	at, err := time.ParseInLocation(timeLayout, s[:len(timeLayout)], time.Local)
	return at, err == nil
}

// ScanQuests — состояние заданий по всем сессиям логов: режим → id задания → последнее событие
// ("start" | "fail" | "done"). Режим уведомления — по последней строке Session mode перед ним
// в той же сессии. Задания, взятые раньше самой старой сохранённой сессии, не видны.
func ScanQuests(logsDir string) map[string]map[string]string {
	out := map[string]map[string]string{}
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return out
	}
	// события всех сессий — по времени: по имени папки порядок неверен, час в нём без
	// ведущего нуля (log_2026.10.30_9-00-00_… по строке позже log_2026.10.30_10-00-00_…)
	var all []Event
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var evs []Event
		dir := filepath.Join(logsDir, entry.Name())
		for _, f := range logFiles(dir, "*application_*.log") {
			eachLine(f, func(s string) {
				if e, ok := ParseLine(s); ok && e.Kind == EvMode {
					evs = append(evs, e)
				}
			})
		}
		for _, f := range logFiles(dir, "*push-notifications_*.log") {
			var r questReader
			eachLine(f, func(s string) {
				if e, ok := r.line(s); ok {
					evs = append(evs, e)
				}
			})
		}
		byTime(evs)
		mode := ""
		for _, e := range evs {
			switch {
			case e.Kind == EvMode:
				mode = e.Mode
			case mode != "":
				e.Mode = mode
				all = append(all, e)
			}
		}
	}
	byTime(all)
	for _, e := range all {
		if out[e.Mode] == nil {
			out[e.Mode] = map[string]string{}
		}
		out[e.Mode][e.QuestID] = e.Status
	}
	return out
}

func byTime(evs []Event) {
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].At.Before(evs[j].At) })
}

// logFiles — файлы сессии по шаблону имени, по порядку (не filepath.Glob: скобки в пути к игре
// он принял бы за шаблон).
func logFiles(dir, pattern string) []string {
	var files []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if ok, _ := filepath.Match(pattern, e.Name()); ok && !e.IsDir() {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files
}

func eachLine(path string, fn func(string)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		fn(strings.TrimRight(sc.Text(), "\r"))
	}
}
