// Связка окна с Go: события "snapshot" и "view", фильтр, этажи, настройки.
(() => {
  const go = () => window.go.main.App;
  const $ = (s) => document.querySelector(s);

  const FILTER_DEFAULTS = {
    position: true, trail: false, extract_own: true, extract_shared: true, extract_other: false, transit: true, labels: true,
    // вкладка «Лут и места»: по умолчанию только названия мест
    places: true, c_weapon: false, c_med: false, c_tech: false, c_safe: false, c_cache: false, c_body: false, c_misc: false,
    loot_all: false, locks: false, switches: false, hazards: false, weapons: false, bosses: false,
    spawn_pmc: false, spawn_scav: false, spawn_sniper: false, btr: false,
    // вкладка «Задания»
    quests: true, quests_active: true,
  };
  const MODE = { pve: "PvE", regular: "PvP" };

  let side = "pmc";
  let filters = { ...FILTER_DEFAULTS };
  let snapshot = null;
  let view = null;
  let mapID = null;   // карта, чья подложка сейчас загружена
  let shotCount = 0;
  let content = null;    // метки вкладки «Лут и места» (GetMapContent)
  let contentKey = "";   // MapID#ContentGen загруженных меток

  function floorName(name) {
    if (!name) return "Основной";
    const m = /^(\d+)(st|nd|rd|th) Floor$/i.exec(name);
    if (m) return `${m[1]}-й этаж`;
    if (/^Underground/i.test(name)) return "Подземный уровень";
    const known = { Basement: "Подвал", Bunkers: "Бункеры", Garage: "Гараж", Tunnels: "Тоннели" };
    return known[name] || name;
  }

  // --- сторона и фильтр ---
  function applySide() {
    document.querySelectorAll("[name=side]").forEach((r) => (r.checked = r.value === side));
    const other = side === "pmc" ? "scav" : "pmc";
    $("[data-own]").style.background = `var(--${side})`;
    $("[data-other]").style.background = `var(--${other})`;
  }

  function applyFilters() {
    document.querySelectorAll("[data-f]").forEach((cb) => {
      cb.checked = !!filters[cb.dataset.f];
      TMap.setFilter(cb.dataset.f, cb.checked);
    });
  }

  document.querySelectorAll("[name=side]").forEach((r) =>
    r.addEventListener("change", () => {
      side = r.value;
      applySide();
      if (view) TMap.render(view, side);
      go().SetSide(side).catch(console.error);
    }),
  );
  document.querySelectorAll("[data-f]").forEach((cb) =>
    cb.addEventListener("change", () => {
      filters[cb.dataset.f] = cb.checked;
      TMap.setFilter(cb.dataset.f, cb.checked);
      if (cb.dataset.f === "quests_active") applyQuests();
      go().SetFilters(filters).catch(console.error);
    }),
  );

  // --- этажи ---
  function buildFloors(layers) {
    const box = $("#floors");
    box.replaceChildren();
    $("#floors-group").hidden = layers.length < 2;
    for (const l of layers) {
      const lab = document.createElement("label");
      const r = document.createElement("input");
      r.type = "radio";
      r.name = "floor";
      r.value = l.ID;
      r.addEventListener("change", () => selectFloor(l.ID));
      lab.append(r, floorName(l.Name));
      box.appendChild(lab);
    }
  }

  function selectFloor(id) {
    TMap.setFloor(id);
    document.querySelectorAll("[name=floor]").forEach((r) => (r.checked = r.value === id));
    showLootFound();
  }
  TMap.onFloor(selectFloor); // Ctrl + колесо и ссылки «показать» в карточке

  // --- метки вкладки «Лут и места» ---
  function fillCounts(c) {
    const n = {};
    if (c) {
      for (const x of c.Containers) n["c_" + x.Group] = (n["c_" + x.Group] || 0) + 1;
      n.loot_all = c.Loot.length;
      n.locks = c.Locks.length;
      n.switches = c.Switches.length;
      n.hazards = c.Hazards.length;
      n.weapons = c.Weapons.length;
      n.bosses = new Set(c.Bosses.map((b) => b.Name)).size;
      for (const s of c.Spawns) n["spawn_" + s.Kind] = (n["spawn_" + s.Kind] || 0) + 1;
      n.btr = c.BTR.length;
    }
    document.querySelectorAll("[data-n]").forEach((s) => {
      const v = c ? n[s.dataset.n] || 0 : null;
      s.textContent = v === null ? "" : v;
      s.closest("label").classList.toggle("empty", v === 0);
    });
  }

  function dropContent() {
    content = null;
    contentKey = "";
    fillCounts(null);
    applyQuests();
    // поле выключают только метки карты без названий; нет меток — нет и причины
    lootQ.disabled = false;
    lootQ.placeholder = LOOT_PH;
    $("#loot-found").textContent = "";
  }

  // --- поиск лута на земле ---
  const lootQ = $("#loot-q");
  const LOOT_PH = lootQ.placeholder;
  let lootTimer = 0;

  // 1 место, 3 места, 5 мест, 11 мест, 21 место
  function places(n) {
    const d = n % 10, dd = n % 100;
    if (d === 1 && dd !== 11) return n + " место";
    if (d >= 2 && d <= 4 && (dd < 12 || dd > 14)) return n + " места";
    return n + " мест";
  }

  // строка остаётся в поле, даже когда оно неактивно: появятся названия — поиск вернётся
  let lootN = 0; // найдено на всех этажах: точки лута, контейнеры, замки

  function applyLootQuery() {
    lootN = TMap.setLootQuery(lootQ.disabled ? "" : lootQ.value);
    showLootFound();
  }

  // места других этажей скрыты — сказать, сколько из найденных видно на этом
  function showLootFound() {
    const q = lootQ.disabled ? "" : lootQ.value;
    const searching = q.split(",").some((w) => w.trim().length >= 2);
    let text = !searching ? "" : lootN ? "найдено " + places(lootN) : "не найдено";
    const off = text && lootN ? TMap.lootHitsElsewhere() : 0;
    if (off) text = `найдено ${lootN}, на этаже ${lootN - off}`;
    if (lootQ.disabled && lootQ.value.trim()) text = "нет названий предметов"; // placeholder закрыт строкой
    $("#loot-found").textContent = text;
  }

  function saveLootQuery() {
    go().SetLootQuery(lootQ.value).catch(console.error);
  }

  lootQ.addEventListener("input", () => {
    clearTimeout(lootTimer);
    lootTimer = setTimeout(() => {
      applyLootQuery();
      saveLootQuery();
    }, 150);
  });
  $("#loot-clear").addEventListener("click", () => {
    clearTimeout(lootTimer);
    lootQ.value = "";
    applyLootQuery();
    saveLootQuery();
  });

  // loadContent берёт метки, если сменилась карта или данные; false — ответ уже
  // устарел (карта сменилась между вызовами), вид надо перечитать
  async function loadContent(v) {
    const key = v.MapID + "#" + v.ContentGen;
    if (key === contentKey) return true;
    let c;
    try {
      c = await go().GetMapContent();
    } catch (e) {
      // метки — дополнение: без них карта, выходы и маршрут работают; следующий вид попробует снова
      console.error(e);
      dropContent();
      return true;
    }
    if (c.MapID !== v.MapID || c.Gen !== v.ContentGen) return false;
    for (const k of ["Places", "Containers", "Loot", "Locks", "Switches", "Hazards", "Weapons", "Bosses", "Spawns", "BTR", "Tasks"]) c[k] = c[k] || [];
    c.Items = c.Items || {};
    content = c;
    contentKey = key;
    lootQ.disabled = c.ItemsMissing;
    lootQ.placeholder = c.ItemsMissing ? "В этой версии программы нет названий предметов" : LOOT_PH;
    applyQuests(); // до setContent: слой заданий строится сразу с выбором этой карты
    lootN = TMap.setContent(c, lootQ.disabled ? "" : lootQ.value);
    fillCounts(c);
    showLootFound();
    return true;
  }

  // --- вкладка «Задания» ---
  // На карте — отмеченные задания; «Только активные» оставляет взятые и не сданные по логам игры.
  // Логов нет или в них нет заданий этого режима — фильтру не на что опереться, видны все.
  const QUEST_STATUS = { start: "взято", done: "сдано", fail: "провалено" };
  const questRank = (t) => ({ start: 0, done: 2, fail: 2 })[questStatus[t.ID]] ?? 1; // взятые наверху, сданные внизу
  let questStatus = {};          // id → "start" | "done" | "fail"
  let hiddenTasks = new Set();   // id снятых галочек — из настроек
  const questQ = $("#quest-q"), questList = $("#quests");

  const knownStatus = () => Object.keys(questStatus).length > 0;
  const passesActive = (t) => !filters.quests_active || !knownStatus() || questStatus[t.ID] === "start";

  // listed — задания в списке: фильтр «Только активные» и поиск по названию
  function listed() {
    if (!content) return [];
    const q = questQ.value.trim().toLowerCase().replaceAll("ё", "е");
    return content.Tasks.filter((t) => passesActive(t) && (!q || t.Name.toLowerCase().replaceAll("ё", "е").includes(q)))
      .sort((a, b) => questRank(a) - questRank(b));
  }

  function applyQuests() {
    const tasks = content ? content.Tasks : [];
    const on = tasks.filter((t) => passesActive(t) && !hiddenTasks.has(t.ID));
    TMap.setQuests(new Set(on.map((t) => t.ID)), questStatus);
    let sum = content ? `На карте ${on.length} из ${tasks.length}` : "";
    if (content && filters.quests_active && !knownStatus()) sum += " · в логах игры заданий нет, видны все";
    $("#quest-sum").textContent = sum;
    renderQuestList();
  }

  function renderQuestList() {
    const top = questList.scrollTop;
    questList.replaceChildren();
    const add = (parent, tag, cls, text) => {
      const n = document.createElement(tag);
      if (cls) n.className = cls;
      if (text) n.textContent = text;
      parent.appendChild(n);
      return n;
    };
    const tasks = listed();
    if (!tasks.length) {
      let text = "На этой карте заданий нет";
      if (content && content.Tasks.length) {
        text = questQ.value.trim() ? "Ничего не найдено" : "Взятых заданий на этой карте нет — снимите «Только активные», чтобы увидеть все";
      }
      add(questList, "div", "empty", text);
      return;
    }
    for (const t of tasks) {
      const box = add(questList, "div", "task" + (hiddenTasks.has(t.ID) ? " off" : ""));
      const lab = add(box, "label");
      const cb = add(lab, "input");
      cb.type = "checkbox";
      cb.checked = !hiddenTasks.has(t.ID);
      cb.addEventListener("change", () => {
        if (cb.checked) hiddenTasks.delete(t.ID);
        else hiddenTasks.add(t.ID);
        box.classList.toggle("off", !cb.checked);
        saveHidden();
      });
      const name = add(lab, "span", "name", t.Name);
      const st = questStatus[t.ID];
      if (st) add(name, "span", "st " + st, QUEST_STATUS[st]);
      const meta = [t.Trader, t.Kappa ? "Каппа" : ""].filter(Boolean).join(" · ");
      if (meta) add(box, "div", "meta", meta);
      for (const o of t.Objectives) {
        const p = o.Zones[0] || o.Points[0];
        const row = add(box, "div", "obj" + (o.Optional ? " opt" : "") + (p ? " lnk" : ""), o.Text + (o.Optional ? " (необяз.)" : ""));
        if (p) {
          row.title = "Показать на карте";
          row.addEventListener("click", () => TMap.showPoint(p.PX, p.PY, p.Layer));
        }
      }
    }
    questList.scrollTop = top;
  }

  function saveHidden() {
    applyQuests();
    go().SetHiddenTasks([...hiddenTasks]).catch(console.error);
  }

  // «Все» и «Ничего» — для заданий в списке (с учётом поиска), остальные не трогаются
  $("#quests-all").addEventListener("click", () => {
    for (const t of listed()) hiddenTasks.delete(t.ID);
    saveHidden();
  });
  $("#quests-none").addEventListener("click", () => {
    for (const t of listed()) hiddenTasks.add(t.ID);
    saveHidden();
  });
  questQ.addEventListener("input", renderQuestList);

  function loadQuestStatus() {
    return go().GetQuestStatus().then((s) => {
      questStatus = s || {};
      applyQuests();
    }).catch(console.error);
  }

  // список виден на вкладке «Задания», пока лента развёрнута и карта на экране
  function showQuestList() {
    const tab = document.querySelector(".tab.active[data-tab]");
    questList.hidden = !(tab && tab.dataset.tab === "quests" && !ribbon.classList.contains("collapsed") && view);
  }

  // --- вид карты ---
  let busy = false, again = false;
  async function loadView() {
    if (busy) { again = true; return; }
    busy = true;
    try {
      do { again = false; await updateView(); } while (again);
    } catch (e) {
      console.error(e);
    } finally {
      busy = false;
    }
  }

  async function updateView() {
    const v = await go().GetView();
    v.Markers = v.Markers || [];
    v.Layers = v.Layers || [];
    if (!v.Supported) {
      view = null;
      mapID = null;
      viewError = "";
      TMap.clear();
      dropContent();
      showMessage();
      showQuestList();
      return;
    }
    if (v.MapID !== mapID) {
      let text;
      try {
        text = await go().GetSVG(v.MapID);
      } catch (e) {
        view = null;
        mapID = null;
        viewError = "Не удалось загрузить карту: " + e;
        TMap.clear();
        dropContent();
        showMessage();
        showQuestList();
        return;
      }
      viewError = "";
      mapID = v.MapID;
      shotCount = 0;
      $("#map").hidden = false;
      $("#msg").hidden = true;
      TMap.setSVG(text, v.Rect, v.Layers); // сбрасывает и метки
      dropContent();
      buildFloors(v.Layers);
      selectFloor(v.Layers.length ? v.Layers[0].ID : "");
    }
    if (!(await loadContent(v))) {
      again = true;
      return;
    }
    view = v;
    TMap.render(v, side);
    applyFilters();
    showQuestList();
    const shots = v.Markers.filter((m) => m.Kind === "shot");
    // новый скрин — показать этаж, на котором он сделан, и при слежении удержать его в кадре
    if (shots.length > shotCount) {
      const last = shots[shots.length - 1];
      selectFloor(last.Layer);
      if ($("#follow").checked) TMap.keepInView(last.PX, last.PY);
    }
    shotCount = shots.length;
    $("#center").disabled = shots.length === 0;
    $("#fit").disabled = false;
  }

  // ошибка подложки живёт до следующей удачной загрузки: обновления статуса её не затирают
  let viewError = "";
  function showMessage() {
    $("#map").hidden = true;
    $("#msg").hidden = false;
    $("#msg").textContent = viewError || (snapshot && snapshot.Message) || "Ждём загрузки карты в игре";
    $("#center").disabled = true;
    $("#fit").disabled = true;
    buildFloors([]);
  }

  $("#fit").addEventListener("click", () => TMap.fit());

  $("#center").addEventListener("click", () => {
    const shots = view ? view.Markers.filter((m) => m.Kind === "shot") : [];
    if (!shots.length) return;
    TMap.centerOn(shots[shots.length - 1].PX, shots[shots.length - 1].PY);
    $("#follow").checked = true;
  });

  // --- ручной выбор карты ---
  const mapPick = $("#map-pick");
  let filling = null;

  // список заполняется, когда данные карт уже есть (при первом запуске их качают на старте)
  function fillMapPick() {
    if (mapPick.options.length > 1 || filling) return filling || Promise.resolve();
    filling = go().GetMaps().then((maps) => {
      for (const m of maps || []) {
        const o = new Option(m.Name, m.Scene);
        o.dataset.id = m.ID;
        mapPick.add(o);
      }
      syncMapPick();
    }).finally(() => (filling = null));
    return filling;
  }

  function syncMapPick() {
    const s = snapshot || {};
    const o = s.Manual ? [...mapPick.options].find((x) => x.dataset.id === s.MapID) : null;
    mapPick.value = o ? o.value : "";
  }

  // не ждать тика бэкенда (до секунды): снимок и вид — сразу
  mapPick.addEventListener("change", async () => {
    try {
      await go().SelectMap(mapPick.value);
      snapshot = await go().GetSnapshot();
      await updateStatus();
      await loadView();
    } catch (e) {
      console.error(e);
    }
  });

  // --- статусная строка ---
  function fmtDate(iso) {
    const d = new Date(iso);
    if (isNaN(d) || d.getFullYear() < 2000) return "";
    return d.toLocaleString("ru-RU", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
  }

  // --- часы Таркова и таймер рейда ---
  // Время в Таркове идёт в 7 раз быстрее реального и по Москве (UTC+3); вторые часы — на 12 часов
  // позже (их выбирают при входе в рейд). На Заводе время стоит: днём 15:28, ночью 03:28.
  const HOUR = 3600e3;
  const FIXED_TIME = { factory: "15:28", "night-factory": "03:28" };
  const two = (n) => String(n).padStart(2, "0");
  function tarkovTime(ms, late) {
    const t = (ms * 7 + 3 * HOUR + (late ? 12 * HOUR : 0)) % (24 * HOUR);
    return two(Math.floor(t / HOUR)) + ":" + two(Math.floor((t % HOUR) / 60e3));
  }
  const when = (iso) => { const t = Date.parse(iso); return t > 0 ? t : 0; }; // нулевое время Go — 0001 год

  function tick() {
    const s = snapshot || {}, now = Date.now(), clock = $("#clock");
    clock.replaceChildren();
    const tt = document.createElement("span");
    tt.textContent = FIXED_TIME[s.MapID] || tarkovTime(now, false) + " / " + tarkovTime(now, true);
    tt.title = FIXED_TIME[s.MapID] ? "Время в Таркове: на Заводе оно не идёт" : "Время в Таркове: дневной и ночной рейд";
    clock.append(tt);
    const start = when(s.RaidStart);
    if (start && !when(s.RaidEnd) && s.RaidMinutes > 0) {
      const left = Math.round((start + s.RaidMinutes * 60e3 - now) / 1000), r = document.createElement("b");
      r.textContent = left > 0 ? `до конца рейда ${Math.floor(left / 60)}:${two(left % 60)}` : "время рейда вышло";
      r.className = left > 300 ? "" : "warn";
      r.title = `Рейд на этой карте — ${s.RaidMinutes} мин; отсчёт от начала рейда в логе игры`;
      clock.append(" · ", r);
    }
    clock.append(" · ");
  }
  setInterval(tick, 1000);

  async function updateStatus() {
    const s = snapshot || {};
    const [p, updated] = await Promise.all([go().GetPaths(), go().GetDataUpdated()]);
    tick();
    const st = $("#status-main");
    st.replaceChildren();
    const add = (text, cls, title) => {
      if (st.childNodes.length) st.append(" · ");
      const n = document.createElement(cls === "b" ? "b" : "span");
      if (cls && cls !== "b") n.className = cls;
      if (title) n.title = title;
      n.textContent = text;
      st.appendChild(n);
    };
    if (MODE[s.Mode]) add(MODE[s.Mode], "b");
    if (s.MapName) add(s.MapName, "b");
    if (s.Manual) add("выбрана вручную");
    syncMapPick();
    fillMapPick().catch(console.error);
    if (s.LastShot) add("последний скрин " + s.LastShot);
    add(p.GameFound ? "игра найдена" : "игра не найдена", p.GameFound ? "" : "bad", p.Game || "путь не задан");
    add(p.ShotsFound ? "скрины найдены" : "скрины не найдены", p.ShotsFound ? "" : "bad", p.Shots || "путь не задан");
    const d = fmtDate(updated);
    add(d ? "данные от " + d : "данных нет", d ? "" : "bad");
    if (s.Message) add(s.Message, "bad");
    $("#data-date").textContent = d ? "Данные от " + d : "Данных нет";
    if (!view) showMessage();
  }

  // --- лента: вкладки, сворачивание двойным щелчком ---
  const ribbon = $("#ribbon");
  function selectTab(name) {
    document.querySelectorAll(".tab[data-tab]").forEach((t) => {
      t.classList.toggle("active", t.dataset.tab === name);
      t.setAttribute("aria-selected", t.dataset.tab === name);
    });
    document.querySelectorAll(".pane").forEach((p) => p.classList.toggle("active", p.dataset.pane === name));
    if (name === "settings") fillSettings().catch(console.error);
    showQuestList();
  }
  function setCollapsed(on) {
    ribbon.classList.toggle("collapsed", on);
    $("#collapse").textContent = on ? "˅" : "˄";
    $("#collapse").title = on ? "Развернуть ленту" : "Свернуть ленту";
    showQuestList();
    try { localStorage.setItem("ribbonCollapsed", on ? "1" : ""); } catch (e) { /* без хранилища — не запоминаем */ }
  }
  let justExpanded = false; // двойной щелчок по свёрнутой ленте оставляет её развёрнутой
  document.querySelectorAll(".tab[data-tab]").forEach((t) => {
    t.addEventListener("click", (e) => {
      if (e.detail === 2) {
        if (!justExpanded) setCollapsed(true);
        justExpanded = false;
        return;
      }
      justExpanded = ribbon.classList.contains("collapsed");
      if (justExpanded) setCollapsed(false);
      selectTab(t.dataset.tab);
    });
  });
  $("#collapse").addEventListener("click", () => setCollapsed(!ribbon.classList.contains("collapsed")));
  // в узком окне лента не помещается и прокручивается вбок — колесом мыши тоже
  document.querySelectorAll(".pane").forEach((p) => p.addEventListener("wheel", (ev) => {
    if (p.scrollWidth <= p.clientWidth || ev.deltaX) return;
    p.scrollLeft += ev.deltaY;
    ev.preventDefault();
  }, { passive: false }));
  let collapsed = false;
  try { collapsed = localStorage.getItem("ribbonCollapsed") === "1"; } catch (e) { /* по умолчанию развёрнута */ }
  setCollapsed(collapsed);
  selectTab("main");

  // --- вкладка «Настройки» ---
  async function fillSettings() {
    const [st, p] = await Promise.all([go().GetSettings(), go().GetPaths()]);
    $("#game-dir").value = st.GameDir || "";
    $("#shots-dir").value = st.ShotsDir || "";
    $("#game-dir").placeholder = "авто: " + (p.DetectedGame || "не найдено");
    $("#shots-dir").placeholder = "авто: " + (p.DetectedShots || "не найдено");
    $("#settings-result").textContent = "";
  }
  $("#save-paths").addEventListener("click", async () => {
    const res = $("#settings-result");
    try {
      await go().SetPaths($("#game-dir").value, $("#shots-dir").value);
      const p = await go().GetPaths();
      res.textContent = `Игра: ${p.GameFound ? "найдена" : "не найдена"} · скрины: ${p.ShotsFound ? "найдены" : "не найдены"}`;
      updateStatus().catch(console.error);
    } catch (e) {
      res.textContent = "Ошибка: " + e;
    }
  });

  // --- запуск ---
  window.addEventListener("load", async () => {
    go().GetVersion().then((v) => ($("#version").textContent = "Tarkov Map " + v)).catch(console.error);
    try {
      const st = await go().GetSettings();
      side = st.Side === "scav" ? "scav" : "pmc";
      // только известные ключи: устаревшие (например, прежний "route") отбрасываются
      const saved = st.Filters || {};
      for (const k in FILTER_DEFAULTS) filters[k] = k in saved ? !!saved[k] : FILTER_DEFAULTS[k];
      lootQ.value = st.LootQuery || "";
      hiddenTasks = new Set(st.HiddenTasks || []);
    } catch (e) {
      console.error(e);
    }
    applySide();
    applyFilters();
    window.runtime.EventsOn("snapshot", (s) => {
      snapshot = s;
      updateStatus().catch(console.error);
    });
    window.runtime.EventsOn("view", () => loadView());
    window.runtime.EventsOn("quests", () => loadQuestStatus());
    await loadQuestStatus();
    snapshot = await go().GetSnapshot();
    await loadView();
    await updateStatus();
  });
})();
