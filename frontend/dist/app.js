// Связка окна с Go: события "snapshot" и "view", фильтр, этажи, настройки.
(() => {
  const go = () => window.go.main.App;
  const $ = (s) => document.querySelector(s);

  const FILTER_DEFAULTS = { position: true, trail: false, extract_own: true, extract_shared: true, extract_other: false, transit: true };
  const MODE = { pve: "PvE", regular: "PvP" };

  let side = "pmc";
  let filters = { ...FILTER_DEFAULTS };
  let snapshot = null;
  let view = null;
  let mapID = null;   // карта, чья подложка сейчас загружена
  let shotCount = 0;

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
      r.addEventListener("change", () => TMap.setFloor(l.ID));
      lab.append(r, floorName(l.Name));
      box.appendChild(lab);
    }
  }

  function selectFloor(id) {
    TMap.setFloor(id);
    document.querySelectorAll("[name=floor]").forEach((r) => (r.checked = r.value === id));
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
    if (!v.Supported) {
      view = null;
      mapID = null;
      TMap.clear();
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
        TMap.clear();
        showMessage("Не удалось загрузить карту: " + e);
        return;
      }
      mapID = v.MapID;
      shotCount = 0;
      $("#map").hidden = false;
      $("#msg").hidden = true;
      TMap.setSVG(text, v.Rect, v.Layers);
      buildFloors(v.Layers);
      selectFloor(v.Layers.length ? v.Layers[0].ID : "");
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

  function showMessage(text) {
    $("#map").hidden = true;
    $("#msg").hidden = false;
    $("#msg").textContent = text || (snapshot && snapshot.Message) || "Ждём загрузки карты в игре";
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
  // сдвинули карту мышью — смотрят в другое место, слежение выключаем
  TMap.setOnPan(() => ($("#follow").checked = false));

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
  $("#refresh-data").addEventListener("click", async () => {
    const res = $("#settings-result");
    res.textContent = "Обновляем…";
    try {
      await go().RefreshData();
      mapID = null; // перечитать подложку
      res.textContent = "Данные обновлены";
      loadView();
    } catch (e) {
      res.textContent = String(e);
    }
    updateStatus().catch(console.error);
  });

  // --- запуск ---
  window.addEventListener("load", async () => {
    try {
      const st = await go().GetSettings();
      side = st.Side === "scav" ? "scav" : "pmc";
      // только известные ключи: устаревшие (например, прежний "route") отбрасываются
      const saved = st.Filters || {};
      for (const k in FILTER_DEFAULTS) filters[k] = k in saved ? !!saved[k] : FILTER_DEFAULTS[k];
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
