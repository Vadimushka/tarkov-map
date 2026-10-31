// Package gamelog читает логи EFT: режим, загрузку карты, начало и конец рейда.
package gamelog

import (
	"strings"
	"time"
)

type Kind int

const (
	EvMode Kind = iota + 1
	EvMap
	EvRaidStart
	EvRaidEnd // рейд окончен: выход в меню или убежище
	EvQuest   // задание взято, провалено или сдано (push-notifications)
)

// Event — значимая строка лога.
type Event struct {
	Kind    Kind
	At      time.Time // из префикса строки, локальное время
	Mode    string    // "pve" | "regular" (EvMode)
	Bundle  string    // "maps/shopping_mall.bundle" (EvMap)
	QuestID string    // EvQuest
	Status  string    // EvQuest: "start" | "fail" | "done"
}

const timeLayout = "2006-01-02 15:04:05.000"

// ParseLine распознаёт строку лога application_*.log.
func ParseLine(line string) (Event, bool) {
	if len(line) < len(timeLayout) {
		return Event{}, false
	}
	at, err := time.ParseInLocation(timeLayout, line[:len(timeLayout)], time.Local)
	if err != nil {
		return Event{}, false
	}
	if i := strings.Index(line, "Session mode: "); i >= 0 {
		switch strings.ToLower(strings.TrimSpace(line[i+len("Session mode: "):])) {
		case "pve":
			return Event{Kind: EvMode, At: at, Mode: "pve"}, true
		case "regular":
			return Event{Kind: EvMode, At: at, Mode: "regular"}, true
		}
		return Event{}, false
	}
	if i := strings.Index(line, "scene preset path:"); i >= 0 {
		rest := line[i+len("scene preset path:"):]
		if j := strings.IndexByte(rest, ' '); j >= 0 {
			rest = rest[:j]
		}
		if rest == "" {
			return Event{}, false
		}
		return Event{Kind: EvMap, At: at, Bundle: rest}, true
	}
	if strings.Contains(line, "|GameStarted:") {
		return Event{Kind: EvRaidStart, At: at}, true
	}
	// пишется при каждом возврате в меню (и при входе в игру — до первой карты)
	if strings.Contains(line, "PrepareSelectedProfileLocally") {
		return Event{Kind: EvRaidEnd, At: at}, true
	}
	return Event{}, false
}
