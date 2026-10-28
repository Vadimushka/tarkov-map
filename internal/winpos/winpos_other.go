//go:build !windows

package winpos

import "github.com/Vadimushka/tarkov-map/internal/settings"

func Get() (settings.WindowPos, bool) { return settings.WindowPos{}, false }

func Apply(settings.WindowPos) bool { return false }
