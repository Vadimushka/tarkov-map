package gamelog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// note — блок push-notifications, как его пишет игра 1.3: строка «Got notification», JSON с отступом
// и строка обработки с тем же временем.
func note(at string, typ int, template string) string {
	return at + "|1.3.0.0.48484|Info|push-notifications|Got notification | ChatMessageReceived\n" +
		"{\n  \"type\": \"new_message\",\n  \"message\": {\n    \"type\": " + fmt.Sprint(typ) + ",\n" +
		"    \"templateId\": \"" + template + "\",\n    \"items\": {\n      \"data\": []\n    }\n  }\n}\n" +
		at + "|1.3.0.0.48484|Info|push-notifications|NotificationMessageProcessor.ProcessMessage | Received notification: Type: ChatMessageReceived\n"
}

const ping = "2026-10-29 21:03:18.943|1.3.0.0.48484|Info|push-notifications|NotificationMessageProcessor.ProcessMessage | Received notification: Type: Ping\n"

func quests(evs []Event) string {
	var out []string
	for _, e := range evs {
		if e.Kind == EvQuest {
			out = append(out, e.QuestID+":"+e.Status)
		}
	}
	return strings.Join(out, ",")
}

func TestQuestNotifications(t *testing.T) {
	log := ping +
		note("2026-10-29 21:32:27.235", 10, "q1 description") +
		note("2026-10-29 21:40:00.000", 4, "q1 description") + // обычное сообщение торговца
		note("2026-10-29 21:45:10.500", 12, "q1 successMessageText") +
		note("2026-10-29 21:50:00.000", 11, "q2 failMessageText")
	var r questReader
	var evs []Event
	for _, l := range strings.Split(log, "\n") {
		if e, ok := r.line(l); ok {
			evs = append(evs, e)
		}
	}
	if got := quests(evs); got != "q1:start,q1:done,q2:fail" {
		t.Fatalf("задания = %s", got)
	}
	if want := time.Date(2026, 10, 29, 21, 45, 10, 500e6, time.Local); !evs[1].At.Equal(want) {
		t.Errorf("время = %v, want %v", evs[1].At, want)
	}
}

func mkLog(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(dir, name)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// Состояние заданий по всем сессиям: режим — по последней строке Session mode перед уведомлением,
// последнее событие побеждает.
func TestScanQuests(t *testing.T) {
	logs := t.TempDir()
	a := filepath.Join(logs, "log_2026.10.28_19-45-54_1.2.5.0.48146")
	mkLog(t, a, "x application_000.log", "2026-10-28 19:46:00.000|1|Info|application|Session mode: Pve\n")
	mkLog(t, a, "x push-notifications_000.log", note("2026-10-28 20:00:00.000", 10, "q1 description")+
		note("2026-10-28 20:10:00.000", 10, "q2 description"))
	b := filepath.Join(logs, "log_2026.10.29_21-00-55_1.3.0.0.48484")
	mkLog(t, b, "x application_000.log", "2026-10-29 21:01:00.000|1|Info|application|Session mode: Regular\n"+
		"2026-10-29 21:30:00.000|1|Info|application|Session mode: Pve\n")
	mkLog(t, b, "x push-notifications_000.log", note("2026-10-29 21:05:00.000", 10, "q3 description")+
		note("2026-10-29 21:35:00.000", 12, "q1 successMessageText"))
	mkLog(t, logs, "не папка.log", "")

	got := ScanQuests(logs)
	want := map[string]map[string]string{
		"pve":     {"q1": "done", "q2": "start"},
		"regular": {"q3": "start"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("ScanQuests = %v, want %v", got, want)
	}
	if n := len(ScanQuests(filepath.Join(logs, "нет"))); n != 0 {
		t.Errorf("без папки логов: %d режимов", n)
	}
}

// Час в имени папки сессии — без ведущего нуля: «_9-00-00» по строке больше «_10-00-00»,
// но события утренней сессии должны идти раньше вечерней.
func TestScanQuestsMorningSession(t *testing.T) {
	logs := t.TempDir()
	a := filepath.Join(logs, "log_2026.10.30_9-00-00_1.3.0.0.48484")
	mkLog(t, a, "x application_000.log", "2026-10-30 09:00:05.000|1|Info|application|Session mode: Pve\n")
	mkLog(t, a, "x push-notifications_000.log", note("2026-10-30 09:10:00.000", 10, "q1 description"))
	b := filepath.Join(logs, "log_2026.10.30_10-00-00_1.3.0.0.48484")
	mkLog(t, b, "x application_000.log", "2026-10-30 10:00:05.000|1|Info|application|Session mode: Pve\n")
	mkLog(t, b, "x push-notifications_000.log", note("2026-10-30 10:10:00.000", 12, "q1 successMessageText"))

	if got := fmt.Sprint(ScanQuests(logs)); got != "map[pve:map[q1:done]]" {
		t.Errorf("ScanQuests = %s, want map[pve:map[q1:done]]", got)
	}
}

// Хвост читает и уведомления; блок, дописанный за два опроса, не теряется.
func TestTailerQuestNotifications(t *testing.T) {
	logs := t.TempDir()
	mkSession(t, logs, "log_a", time.Now().Add(-time.Hour), lMode+"\n")
	full := note("2026-10-28 21:10:00.000", 10, "q1 description")
	cut := strings.Index(full, "\"templateId\"")
	f := mkLog(t, filepath.Join(logs, "log_a"), "x push-notifications_000.log", full[:cut])
	s := &sink{}
	tl := NewTailer(logs, s.add)
	tl.Poll()
	if got := quests(s.take()); got != "" {
		t.Fatalf("половина блока дала %s", got)
	}
	appendTo(t, f, full[cut:])
	tl.Poll()
	if got := quests(s.take()); got != "q1:start" {
		t.Errorf("задания = %q", got)
	}
}
