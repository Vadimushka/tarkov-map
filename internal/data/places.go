package data

import "strings"

// Label — подпись места на карте из maps.json tarkov-dev.
type Label struct {
	Text        string // по-русски, если есть в словаре
	X, Z        float64
	Bottom, Top *float64 // нет в данных — nil
}

// placeRU — русские названия общеизвестных мест; магазины и бренды остаются как в игре.
var placeRU = map[string]string{
	// общие для нескольких карт
	"Power Station": "Электростанция", "Gas Station": "Заправка", "Construction": "Стройка",
	"Cabins": "Домики", "Cottages": "Коттеджи", "Bus Stop": "Остановка", "Crash Site": "Место крушения",
	"Scav Camp": "Лагерь Диких", "Office": "Офис", "Garage": "Гараж",
	// Таможня
	"Big Red": "Красный склад", "Dorms": "Общаги", "New Gas": "Новая заправка", "Old Gas": "Старая заправка",
	"Fortress": "Крепость", "Crackhouse": "Притон", "Streamer House": "Дом стримера", "Main Bridge": "Главный мост",
	"Sniper Hill": "Снайперский холм", "Storage": "Склад", "Trailer Park": "Трейлерный парк",
	"Junk Bridge": "Мусорный мост", "Repair Shop": "Мастерская", "Sniper Ridge": "Снайперский гребень",
	"Old Construction": "Старая стройка", "Skeleton": "Скелет", "Depot": "Депо",
	"Military Checkpoint": "Военный КПП", "Bus Station": "Автостанция", "Powerline Tower": "Опора ЛЭП",
	// Завод
	"Office Building": "Офисы", "Main Office": "Главный офис", "Breach Room": "Комната с проломом",
	"Locked Office": "Закрытый офис", "North Stairs": "Северная лестница", "South Stairs": "Южная лестница",
	"Sky Bridge": "Мостик", "Connector": "Переход", "Locker Rooms": "Раздевалки", "Sinks": "Умывальники",
	"Hole": "Дыра", "Platform": "Платформа", "Servers": "Серверная", "Med Tent": "Медпалатка",
	"Silos": "Силосы", "Heli Crash": "Вертолёт", "Pit": "Яма", "Underground Stash": "Подземный схрон",
	"Scav Bunker": "Бункер Диких", "Blue Containers": "Синие контейнеры", "Wood Room": "Деревянная комната",
	"Glass Hall": "Стеклянный зал", "Boilers": "Котельная", "Pumping Station": "Насосная",
	"East Halls": "Восточные цеха", "Forklifts": "Погрузчики", "Rafters": "Балки",
	// Развязка
	"Go Kart": "Картинг", "Cargo Containers": "Контейнеры", "Wall Break": "Пролом в стене", "Ramps": "Пандусы",
	"Oli Tower": "Башня OLI", "Idea Tower": "Башня IDEA", "Highway Construction": "Стройка трассы",
	// Маяк
	"Train Yard": "Ж/д депо", "Drug Lab": "Нарколаборатория", "Water Treatment": "Очистные сооружения",
	"Pipes": "Трубы", "Gunner Nest": "Пулемётное гнездо", "Island": "Остров", "Convenience": "Магазин",
	"Hillside": "Склон", "Boathouses": "Лодочные сараи", "Dead Tree": "Сухое дерево",
	"Grand Chalet": "Шале", "Tennis Court": "Теннисный корт", "Lightkeeper Island": "Остров Смотрителя",
	// Резерв
	"K Buildings": "Корпуса К", "White Queen": "Белый ферзь", "White Pawn": "Белая пешка",
	"Black Bishop": "Чёрный слон", "White Bishop": "Белый слон", "White King": "Белый король",
	"Black Knight": "Чёрный конь", "White Knight": "Белый конь", "White Rook": "Белая ладья",
	"Black Pawn": "Чёрная пешка", "Train Station": "Вокзал", "Barracks": "Казармы",
	"Mechanic": "Мастерская", "Shipping Yard": "Погрузочная площадка", "Dome": "Купол", "Tarmac": "Аэродром",
	// Берег
	"Resort": "Санаторий", "Weather Station": "Метеостанция", "Radio Tower": "Радиовышка", "Swamp": "Болото",
	"Village": "Деревня", "Tank Bridge": "Танковый мост", "Pier": "Пирс", "Scav Island": "Остров Диких",
	"Bunker": "Бункер", "Crane": "Кран", "Scav Farm": "Ферма Диких", "West Wing": "Западное крыло",
	"East Wing": "Восточное крыло", "Admin": "Администрация",
	// Лес
	"Sawmill": "Лесопилка", "Scav Town": "Посёлок Диких", "Old Sawmill": "Старая лесопилка",
	"Cultist Village": "Деревня сектантов", "USEC Camp": "Лагерь USEC", "Military Camp": "Военный лагерь",
	"Ponds": "Пруды", "Checkpoint": "КПП", "Shack": "Хижина", "Lumber": "Лесосклад",
	"Jaeger's Camp": "Лагерь Егеря", "Sniper Rock": "Снайперская скала", "Convoy": "Колонна",
	// Улицы Таркова
	"Primorsky Ave.": "Приморский пр.", "Kilmov St.": "ул. Климова", "Nikitskaya St.": "ул. Никитская",
	"Verhnyaya St.": "ул. Верхняя", "Malevecha St.": "ул. Малевича", "Chekannaya St.": "ул. Чеканная",
	"Nizhnyaya St.": "ул. Нижняя", "Razvedchikov St.": "ул. Разведчиков", "Zmejskij Alley": "Змейский пер.",
	"Kamchatskaya St.": "ул. Камчатская", "Kilmov Shopping Mall": "ТЦ Климов", "Cinema": "Кинотеатр",
	"Shestyorochka": "Шестёрочка", "Post Office": "Почта", "Corner Restaurant": "Угловой ресторан",
	"School": "Школа", "Vet Clinic": "Ветклиника", "Abandon Factory": "Заброшенный завод", "Diner": "Закусочная",
	"Primorskij 49": "Приморский 49",
	// Эпицентр
	"Science Office": "Научный офис",
}

// placePrefixRU — нумерованные места: «Warehouse 17» → «Склад 17».
var placePrefixRU = [][2]string{{"Warehouse ", "Склад "}, {"Garage ", "Гараж "}, {"Plant ", "Цех "}, {"Pharmacy ", "Аптека "}}

// placeName переводит подпись места; неизвестная остаётся как в игре.
func placeName(text string) string {
	if ru, ok := placeRU[text]; ok {
		return ru
	}
	for _, p := range placePrefixRU {
		if rest, ok := strings.CutPrefix(text, p[0]); ok {
			return p[1] + rest
		}
	}
	return text
}

type rawLabel struct {
	Position []float64 `json:"position"` // [x, z]
	Text     string    `json:"text"`
	Bottom   *float64  `json:"bottom"`
	Top      *float64  `json:"top"`
}

func toLabels(raw []rawLabel) []Label {
	var out []Label
	for _, l := range raw {
		if len(l.Position) < 2 {
			continue
		}
		out = append(out, Label{Text: placeName(l.Text), X: l.Position[0], Z: l.Position[1], Bottom: l.Bottom, Top: l.Top})
	}
	return out
}
