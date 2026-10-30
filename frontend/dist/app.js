// Связка окна с Go: события "snapshot" и "view", фильтр, этажи, настройки.
(() => {
  const go = () => window.go.main.App;
  const $ = (s) => document.querySelector(s);

  const FILTER_DEFAULTS = {
    position: true, trail: false, extract_own: true, extract_shared: true, extract_other: false, transit: true, labels: true,
    // вкладка «Лут и места»: по умолчанию только названия мест
    places: true, c_weapon: false, c_med: false, c_tech: false, c_safe: false, c_cache: false, c_body: false, c_misc: false,
    loot_all: false, locks: false, switches: false, hazards: false, weapons: false, bosses: false,
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
    // поле выключают только метки карты без названий; нет меток — нет и причины
    lootQ.disabled = false;
    lootQ.placeholder = LOOT_PH;
    $("#loot-found").textContent = "";
  }

  // --- поиск лута на земле ---
  const lootQ = $("#loot-q");
  const LOOT_PH = lootQ.placeholder;
  let lootTimer = 0;

  // 1 точка, 3 точки, 5 точек, 11 точек, 21 точка
  function points(n) {
    const d = n % 10, dd = n % 100;
    if (d === 1 && dd !== 11) return n + " точка";
    if (d >= 2 && d <= 4 && (dd < 12 || dd > 14)) return n + " точки";
    return n + " точек";
  }

  // строка остаётся в поле, даже когда оно неактивно: появятся названия — поиск вернётся
  let lootN = 0; // совпавших точек на всех этажах

  function applyLootQuery() {
    lootN = TMap.setLootQuery(lootQ.disabled ? "" : lootQ.value);
    showLootFound();
  }

  // точки других этажей скрыты — сказать, сколько из найденных видно на этом
  function showLootFound() {
    const q = lootQ.disabled ? "" : lootQ.value;
    const found = lootN % 10 === 1 && lootN % 100 !== 11 ? "найдена " : "найдено "; // найдена 1 точка, найдено 3 точки
    let text = q.trim().length < 2 ? "" : lootN ? found + points(lootN) : "не найдено";
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
    for (const k of ["Places", "Containers", "Loot", "Locks", "Switches", "Hazards", "Weapons", "Bosses"]) c[k] = c[k] || [];
    c.Items = c.Items || {};
    content = c;
    contentKey = key;
    lootQ.disabled = c.ItemsMissing;
    lootQ.placeholder = c.ItemsMissing ? "В этой версии программы нет названий предметов" : LOOT_PH;
    lootN = TMap.setContent(c, lootQ.disabled ? "" : lootQ.value);
    fillCounts(c);
    showLootFound();
    return true;
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

  async function updateStatus() {
    const s = snapshot || {};
    const [p, updated] = await Promise.all([go().GetPaths(), go().GetDataUpdated()]);
    const st = $("#status");
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
  }
  function setCollapsed(on) {
    ribbon.classList.toggle("collapsed", on);
    $("#collapse").textContent = on ? "˅" : "˄";
    $("#collapse").title = on ? "Развернуть ленту" : "Свернуть ленту";
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
    snapshot = await go().GetSnapshot();
    await loadView();
    await updateStatus();
  });
})();
