// Отрисовка карты: SVG-подложка, маркеры по готовым координатам (PX/PY), этажи, масштаб и сдвиг.
// Значки и подписи рисуются в пикселях экрана: при любом масштабе они одного размера.
window.TMap = (() => {
  const NS = "http://www.w3.org/2000/svg";
  const svg = document.getElementById("map");
  const tip = document.getElementById("tip");

  const el = (tag, attrs, parent) => {
    const e = document.createElementNS(NS, tag);
    for (const k in attrs) e.setAttribute(k, attrs[k]);
    if (parent) parent.appendChild(e);
    return e;
  };
  const css = (v) => getComputedStyle(document.documentElement).getPropertyValue(v).trim();

  const base = el("g", {}, svg);
  const overlay = el("g", {}, svg);
  const content = el("g", {}, overlay); // вкладка «Лут и места» — под выходами и маршрутом
  // порядок — снизу вверх: зоны выходов под значками, путь поверх выходов, текущее место поверх всего
  const groups = {};
  for (const k of ["zone", "transit", "extract_other", "extract_shared", "extract_own", "trail", "position"]) groups[k] = el("g", {}, overlay);
  const flashG = el("g", {}, overlay); // кольцо «вот здесь» после ссылки «показать»

  let R = null;          // прямоугольник карты в координатах вида
  let vb = null;         // текущий viewBox
  let floorGroups = [];  // этажи внутри SVG
  let baseLayer = "", floor = "";
  let layerIds = new Set(); // этажи кроме базового
  let floorOrder = [];   // этажи снизу вверх — для Ctrl + колесо
  let markers = [];      // выходы и переходы последней отрисовки — для вписывания и карточек
  let pins = [];         // группы меток в пикселях экрана: {g, x, y}
  let upp = 1;           // единиц карты на пиксель экрана
  let onFloorCb = null;  // смена этажа из карты (Ctrl + колесо, «показать»): app.js отмечает её в ленте

  const FACTION = { pmc: "ЧВК", scav: "Дикие", shared: "Совместный" };
  const EXIT = { pmc: "Выход ЧВК", scav: "Выход Диких", shared: "Совместный выход" };

  function showTip(ev, text) {
    tip.style.display = "block";
    tip.textContent = text;
    // у края окна — по другую сторону курсора, чтобы подсказка не обрезалась
    const w = tip.offsetWidth, h = tip.offsetHeight;
    let x = ev.clientX + 12, y = ev.clientY + 12;
    if (x + w > innerWidth) x = Math.max(0, ev.clientX - 12 - w);
    if (y + h > innerHeight) y = Math.max(0, ev.clientY - 12 - h);
    tip.style.left = x + "px";
    tip.style.top = y + "px";
  }
  // zone — контур выхода или перехода: виден, пока мышь над значком или открыта его карточка
  function hover(node, text, zone) {
    node.addEventListener("mousemove", (ev) => {
      showTip(ev, text);
      if (zone) zone.style.display = "";
    });
    node.addEventListener("mouseleave", () => {
      tip.style.display = "none";
      if (zone && zone !== pinnedZone) zone.style.display = "none";
    });
  }

  // --- карточка по клику ---
  // spec: {title, sub, lines: [строка], links: [{head, items: [{text, x, y, layer}]}], key, zone}
  const card = document.getElementById("card");
  const onClick = new WeakMap(); // метка → действие по клику
  let cardAt = null;     // точка карты, к которой привязана карточка
  let cardKey = "";      // выход в карточке: после перерисовки вида снова показать его зону
  let pinnedZone = null; // зона, которую держит открытая карточка

  function openCard(spec, x, y) {
    closeCard();
    cardAt = { x, y };
    cardKey = spec.key || "";
    pinnedZone = spec.zone || null;
    if (pinnedZone) pinnedZone.style.display = "";
    card.querySelector(".title").textContent = spec.title;
    const body = card.querySelector(".body");
    body.replaceChildren();
    const add = (cls, text) => {
      const d = document.createElement("div");
      if (cls) d.className = cls;
      d.textContent = text;
      body.appendChild(d);
      return d;
    };
    if (spec.sub) add("sub", spec.sub);
    for (const l of spec.lines || []) add("", l);
    for (const sec of spec.links || []) {
      if (!sec.items.length) continue;
      add("head", sec.head);
      for (const it of sec.items) {
        const row = add("lnk", it.text);
        row.title = "Показать на карте";
        row.addEventListener("click", () => showPoint(it.x, it.y, it.layer));
      }
    }
    card.hidden = false;
    positionCard();
  }

  function closeCard() {
    card.hidden = true;
    if (pinnedZone) pinnedZone.style.display = "none";
    pinnedZone = null;
    cardAt = null;
    cardKey = "";
  }
  card.querySelector(".close").addEventListener("click", closeCard);
  addEventListener("keydown", (e) => {
    if (e.key === "Escape") closeCard();
  });

  // карточка справа от метки (у края — слева) и едет вместе с картой
  function positionCard() {
    if (card.hidden || !cardAt || !vb || !svg.clientWidth) return;
    const k = svg.clientWidth / vb.w, w = card.offsetWidth, h = card.offsetHeight;
    let x = (cardAt.x - vb.x) * k + 16;
    const y = (cardAt.y - vb.y) * k - 14;
    if (x + w > svg.clientWidth - 4 && x - 32 - w >= 4) x -= 32 + w;
    card.style.left = x + "px";
    card.style.top = Math.min(Math.max(4, y), svg.clientHeight - h - 4) + "px";
  }

  // showPoint — ссылка «показать»: этаж точки, центр на ней (с дальнего вида — с приближением,
  // иначе центрировать некуда) и кольцо на пару секунд
  const flashes = [];
  function showPoint(x, y, layer) {
    if (layer && layer !== floor && onFloorCb) onFloorCb(layer);
    if (sized() && vb.w > fullView().w / 2.5) zoomAt(x, y, fullView().w / 3 / vb.w, true);
    else centerOn(x, y);
    const p = { g: el("g", { class: "flash" }, flashG), x, y };
    el("circle", { r: 18 }, p.g);
    flashes.push(p);
    place(p);
    setTimeout(() => {
      p.g.remove();
      flashes.splice(flashes.indexOf(p), 1);
    }, 1800);
  }

  // pin — группа в точке карты, её содержимое задаётся в пикселях экрана
  const place = (p) => p.g.setAttribute("transform", `translate(${p.x},${p.y}) scale(${upp})`);
  function pin(parent, x, y, layer) {
    const p = { g: el("g", { "data-layer": layer }, parent), x, y };
    pins.push(p);
    place(p);
    return p.g;
  }

  // --- слои вкладки «Лут и места» ---
  // Слой строится, только когда включён (на Улицах ≈2500 меток), и пересчитывается при
  // масштабе, только пока виден (CSS-переменная масштаба на весь слой оказалась медленнее
  // обхода атрибутов). Подсказки — одна пара обработчиков на все слои.
  const LAYERS = ["hazards", "places", "spawn_scav", "spawn_sniper", "spawn_pmc", "c_misc", "c_body", "c_cache", "c_tech",
    "c_med", "c_safe", "c_weapon", "loot_all", "locks", "switches", "weapons", "btr", "bosses", "loot_hit"];
  // ключ → {g, pins, built, on, upp, cg (кружки группировки), cpins, cupp, cfloor}
  const layers = {};
  for (const k of LAYERS) layers[k] = { g: el("g", { style: "display:none" }, content), pins: [], built: false, on: false, upp: 0, cpins: [] };
  const tipOf = new WeakMap(); // метка → () => текст подсказки
  let data = null;             // MapContent текущей карты
  let lootHits = null;         // Set индексов совпавших точек лута; null — поиска нет

  const LOCK = { door: "Дверь", trunk: "Багажник" };
  const HAZARD = { minefield: "Минное поле", sniper: "Зона снайпера" };
  const SPAWN = { pmc: "Спавн ЧВК", scav: "Спавн Диких", sniper: "Спавн снайпера" };
  const GROUP = { weapon: "Оружейный", med: "Медицина", tech: "Техника", safe: "Сейф или касса", cache: "Тайник", body: "Тело", misc: "Сумка и прочее" };
  const pct = (x) => Math.round(x * 100) + "%";

  content.addEventListener("mousemove", (ev) => {
    let n = ev.target;
    while (n && n !== content && !tipOf.has(n)) n = n.parentNode;
    if (n && n !== content) showTip(ev, tipOf.get(n)());
    else tip.style.display = "none";
  });
  content.addEventListener("mouseleave", () => (tip.style.display = "none"));

  // клик без сдвига карты: по метке — её действие (карточка, приближение), мимо — закрыть карточку
  let dragged = false;
  svg.addEventListener("click", (ev) => {
    if (dragged) return;
    let n = ev.target;
    while (n && n !== svg && !onClick.has(n)) n = n.parentNode;
    if (n && n !== svg) onClick.get(n)();
    else closeCard();
  });

  // значки tarkov.dev (MIT, the-hideout/tarkov-dev): 24 px на экране при любом масштабе, как у них
  const ICON = 24;
  const CONTAINER_ICON = {
    "bank-cash-register": "cash-register", "cash-register-tar2-2": "cash-register", "bank-safe": "safe",
    "civilian-body": "dead-scav", "dead-civilian": "dead-scav", "lab-technician-body": "dead-scav",
    "pmc-body": "dead-scav", "scav-body": "dead-scav", "shturmans-stash": "weapon-box",
    "medical-supply-crate": "crate", "ration-supply-crate": "crate", "technical-supply-crate": "crate",
  };
  const KNOWN = new Set(["buried-barrel-cache", "cash-register", "crate", "dead-scav", "drawer", "duffle-bag",
    "festive-airdrop-supply-crate", "grenade-box", "ground-cache", "jacket", "medbag", "medcase", "pc-block",
    "plastic-suitcase", "safe", "toolbox", "weapon-box", "wooden-ammo-box", "wooden-crate"]);
  function containerIcon(type) {
    const n = CONTAINER_ICON[type] || type;
    return KNOWN.has(n) ? "container_" + n : null;
  }
  function icon(name, parent) {
    return el("image", { href: "icons/" + name + ".png", x: -ICON / 2, y: -ICON / 2, width: ICON, height: ICON }, parent);
  }

  // cpin — метка слоя; cardFn — содержимое карточки по клику
  function cpin(L, x, y, layer, tipFn, cardFn) {
    const p = { g: el("g", { "data-layer": layer }, L.g), x, y, layer };
    L.pins.push(p);
    place(p);
    if (tipFn) tipOf.set(p.g, tipFn);
    if (cardFn) onClick.set(p.g, () => openCard(cardFn(), x, y));
    return p.g;
  }
  const at = (o) => ({ text: o.Name, x: o.PX, y: o.PY, layer: o.Layer }); // ссылка «показать»

  // спавн — ромб цвета стороны, у снайпера с прицелом
  function spawnMark(g, kind) {
    const color = css("--" + kind);
    el("path", { d: "M0,-7 L7,0 L0,7 L-7,0 Z", fill: color, stroke: "rgba(0,0,0,.85)", "stroke-width": 1.5 }, g);
    if (kind === "sniper") el("circle", { r: 2.5, fill: "none", stroke: "#000", "stroke-width": 1.5 }, g);
  }
  function btrMark(g) {
    el("rect", { x: -12, y: -7.5, width: 24, height: 15, rx: 3, fill: "#4f5d3a", stroke: "#fff", "stroke-width": 1.2 }, g);
    el("text", { y: 3.2, "text-anchor": "middle", fill: "#fff", "font-size": 8.5, "font-weight": 700 }, g).textContent = "БТР";
  }

  // рубильник: что включает его, что включает он сам и какие выходы ждут его
  function switchCard(s) {
    const byId = (id) => data.Switches.find((o) => o.ID === id);
    const opens = s.Activates.map(byId).filter(Boolean);
    const other = s.Activates.length - opens.length; // двери и прочее без своей метки
    return {
      title: s.Name, sub: "Рубильник",
      lines: other ? [`Открывает ещё ${other} ${plural(other, "объект", "объекта", "объектов")} без метки`] : [],
      links: [
        { head: "Сначала включить", items: data.Switches.filter((o) => o.Activates.includes(s.ID)).map(at) },
        { head: "Включает", items: opens.map(at) },
        { head: "Открывает выходы", items: markers.filter((m) => m.Kind === "extract" && m.Switches.includes(s.ID)).map(at) },
      ],
    };
  }

  // 1 метка, 3 метки, 5 меток
  function plural(n, one, few, many) {
    const d = n % 10, dd = n % 100;
    if (d === 1 && dd !== 11) return one;
    if (d >= 2 && d <= 4 && (dd < 12 || dd > 14)) return few;
    return many;
  }

  // поиск: слова через запятую, каждое от 2 символов; без регистра, «ё» = «е»; подстрока в полном
  // или кратком названии предмета, в названии контейнера, в названии ключа от замка
  const norm = (s) => (s || "").toLowerCase().replaceAll("ё", "е");
  let lootQ = []; // слова поиска, нормализованные
  const parseQuery = (q) => norm(q).split(",").map((s) => s.trim()).filter((s) => s.length >= 2);
  const hasWord = (s) => !!s && lootQ.some((w) => norm(s).includes(w));
  const matches = (n) => hasWord(n.Name) || hasWord(n.Short);
  let boxHits = new Set(), lockHits = new Set(); // индексы совпавших контейнеров и замков

  function matchLoot() {
    lootHits = null;
    boxHits = new Set();
    lockHits = new Set();
    if (!data || !lootQ.length) return;
    const ids = new Set(Object.keys(data.Items).filter((id) => matches(data.Items[id])));
    lootHits = new Set();
    data.Loot.forEach((pt, i) => {
      if (pt.Items.some((id) => ids.has(id))) lootHits.add(i);
    });
    data.Containers.forEach((c, i) => hasWord(c.Name) && boxHits.add(i));
    data.Locks.forEach((l, i) => hasWord(l.Key) && lockHits.add(i));
  }
  const hitCount = () => (lootHits ? lootHits.size + boxHits.size + lockHits.size : 0);

  function containerPin(L, c) {
    const g = cpin(L, c.PX, c.PY, c.Layer, () => c.Name, () => ({ title: c.Name, sub: "Контейнер · " + (GROUP[c.Group] || GROUP.misc) }));
    const ic = containerIcon(c.Type);
    if (ic) icon(ic, g);
    else el("circle", { r: 5, fill: css("--c-" + c.Group) || css("--c-misc"), stroke: "rgba(0,0,0,.8)", "stroke-width": 1 }, g);
    return g;
  }

  function lockPin(L, l) {
    const kind = LOCK[l.Kind] || "Замок", need = l.Key ? "ключ: " + l.Key : "ключ неизвестен";
    const text = [kind, need];
    if (l.NeedsPower) text.push("нужно питание");
    const lines = [need[0].toUpperCase() + need.slice(1)];
    if (l.NeedsPower) lines.push("Нужно питание");
    const g = cpin(L, l.PX, l.PY, l.Layer, () => text.join(" · "), () => ({ title: kind, lines }));
    icon("lock", g);
    return g;
  }

  // найденное поиском — в жёлтом кольце поверх своего слоя, даже если слой выключен
  const ring = (g) => g.insertBefore(el("circle", { r: ICON / 2 + 3, fill: "none", stroke: css("--loot-hit"), "stroke-width": 3 }), g.firstChild);

  // lootLines — что может появиться в точке, найденное поиском первым; не больше max строк
  function lootLines(pt, max) {
    if (data.ItemsMissing) return ["названия предметов не загружены"];
    const items = pt.Items.map((id) => data.Items[id]).filter(Boolean);
    const names = [...items.filter(matches), ...items.filter((n) => !matches(n))].map((n) => n.Name);
    if (!names.length) return ["названия этих предметов неизвестны"];
    const lines = names.slice(0, max);
    if (names.length > max) lines.push(`ещё ${names.length - max}`);
    return lines;
  }
  const lootTip = (pt) => ["Может появиться:", ...lootLines(pt, 8)].join("\n");
  const lootCard = (pt) => ({ title: "Лут на земле", sub: "Может появиться:", lines: lootLines(pt, 30) });

  function build(key) {
    const L = layers[key];
    tip.style.display = "none"; // метка под курсором могла исчезнуть, а mousemove не придёт
    L.g.replaceChildren();
    L.pins = [];
    L.cpins = [];
    L.cg = null;
    L.built = true;
    L.upp = upp;
    if (!data || !R) return;
    if (key === "places") {
      for (const p of data.Places) {
        const g = cpin(L, p.PX, p.PY, p.Layer);
        g.classList.add("place");
        el("text", { "text-anchor": "middle", y: 4 }, g).textContent = p.Text;
      }
    } else if (key.startsWith("c_")) {
      for (const c of data.Containers) if ("c_" + c.Group === key) containerPin(L, c);
    } else if (key === "loot_all" || key === "loot_hit") {
      const hit = key === "loot_hit";
      data.Loot.forEach((pt, i) => {
        if (hit !== !!(lootHits && lootHits.has(i))) return;
        const g = cpin(L, pt.PX, pt.PY, pt.Layer, () => lootTip(pt), () => lootCard(pt));
        icon("loose_loot", g);
        if (hit) ring(g);
      });
      if (hit) {
        for (const i of boxHits) ring(containerPin(L, data.Containers[i]));
        for (const i of lockHits) ring(lockPin(L, data.Locks[i]));
      }
    } else if (key === "locks") {
      for (const l of data.Locks) lockPin(L, l);
    } else if (key === "switches") {
      for (const s of data.Switches) icon("switch", cpin(L, s.PX, s.PY, s.Layer, () => s.Name, () => switchCard(s)));
    } else if (key === "weapons") {
      for (const w of data.Weapons) icon("stationarygun", cpin(L, w.PX, w.PY, w.Layer, () => w.Name, () => ({ title: w.Name, sub: "Стационарное оружие" })));
    } else if (key === "bosses") {
      for (const b of data.Bosses) {
        icon("spawn_boss", cpin(L, b.PX, b.PY, b.Layer, () => `${b.Name} · появляется в ${pct(b.Chance)} рейдов · это место — ${pct(b.SpotChance)}`,
          () => ({ title: b.Name, sub: "Босс", lines: [`Появляется в ${pct(b.Chance)} рейдов`, `Это место — ${pct(b.SpotChance)}`] })));
      }
    } else if (key.startsWith("spawn_")) {
      const kind = key.slice(6), name = SPAWN[kind];
      for (const s of data.Spawns) {
        if (s.Kind === kind) spawnMark(cpin(L, s.PX, s.PY, s.Layer, () => name, () => ({ title: name })), kind);
      }
    } else if (key === "btr") {
      for (const b of data.BTR) btrMark(cpin(L, b.PX, b.PY, b.Layer, () => "Остановка БТР · " + b.Name, () => ({ title: b.Name, sub: "Остановка БТР" })));
    } else if (key === "hazards") {
      for (const h of data.Hazards) {
        const color = css(h.Kind === "sniper" ? "--sniper" : "--hazard"), text = () => HAZARD[h.Kind] || "Опасная зона";
        if (h.Outline.length >= 3) {
          const poly = el("polygon", {
            points: h.Outline.map((p) => p[0] + "," + p[1]).join(" "), "data-layer": h.Layer,
            fill: color, "fill-opacity": 0.15, stroke: color, "stroke-width": 1.5, "stroke-dasharray": "5 4",
            "vector-effect": "non-scaling-stroke",
          }, L.g);
          tipOf.set(poly, text);
        } else {
          icon("hazard", cpin(L, h.PX, h.PY, h.Layer, text));
        }
      }
    }
    dimNodes(L.g);
    cluster(key);
  }

  // --- группировка: метки ближе CLUSTER_PX на экране сливаются в кружок с числом ---
  // Только своего этажа (чужие скрыты); на предельном приближении — все метки по отдельности.
  // Пересобирается после масштаба (с задержкой: колесо крутят несколькими щелчками) и смены этажа.
  const CLUSTER_PX = 22;
  const CLUSTERED = new Set(["c_misc", "c_body", "c_cache", "c_tech", "c_med", "c_safe", "c_weapon", "loot_all"]);

  function cluster(key) {
    const L = layers[key];
    if (!CLUSTERED.has(key) || !L.built) return;
    if (L.cg) L.cg.remove();
    L.cg = el("g", {}, L.g);
    L.cpins = [];
    L.cupp = upp;
    L.cfloor = floor;
    for (const p of L.pins) p.g.classList.remove("clustered");
    if (!vb || !sized() || vb.w <= (fullView().w / MAX_ZOOM) * 1.001) return;
    // жадно по сетке: метка входит в первую группу, чей центр ближе d, иначе начинает свою
    const d = CLUSTER_PX * upp, cells = new Map(), found = [];
    for (const p of L.pins) {
      if (p.layer !== floor) continue;
      const cx = Math.floor(p.x / d), cy = Math.floor(p.y / d);
      let into = null;
      for (let i = -1; i <= 1 && !into; i++) {
        for (let j = -1; j <= 1 && !into; j++) {
          into = (cells.get(cx + i + "," + (cy + j)) || []).find((c) => Math.hypot(c.x - p.x, c.y - p.y) < d) || null;
        }
      }
      if (into) {
        into.m.push(p);
        continue;
      }
      const c = { x: p.x, y: p.y, m: [p] }, k = cx + "," + cy;
      found.push(c);
      if (!cells.has(k)) cells.set(k, []);
      cells.get(k).push(c);
    }
    const color = key === "loot_all" ? css("--accent") : css("--" + key.replace("_", "-"));
    for (const c of found) {
      if (c.m.length < 2) continue;
      let sx = 0, sy = 0;
      for (const p of c.m) {
        p.g.classList.add("clustered");
        sx += p.x;
        sy += p.y;
      }
      const n = c.m.length, p = { g: el("g", { class: "cluster" }, L.cg), x: sx / n, y: sy / n };
      L.cpins.push(p);
      place(p);
      el("circle", { r: n > 99 ? 13 : 11, fill: "rgba(20,19,15,.88)", stroke: color, "stroke-width": 2 }, p.g);
      el("text", { y: 4, "text-anchor": "middle" }, p.g).textContent = n;
      tipOf.set(p.g, () => `${n} ${plural(n, "метка", "метки", "меток")} рядом — щёлкните, чтобы приблизить`);
      onClick.set(p.g, () => zoomAt(p.x, p.y, 1 / 3, true));
    }
  }

  let clusterTimer = 0;
  function reclusterSoon() {
    clearTimeout(clusterTimer);
    clusterTimer = setTimeout(() => {
      for (const k of CLUSTERED) {
        const L = layers[k];
        if (L.on && L.built && (L.cupp !== upp || L.cfloor !== floor)) cluster(k);
      }
    }, 120);
  }

  function show(key, on) {
    const L = layers[key];
    L.on = on;
    if (on && !L.built) build(key);
    else if (on && L.upp !== upp) {
      L.pins.forEach(place);
      L.cpins.forEach(place);
      L.upp = upp;
    }
    if (on && L.built && CLUSTERED.has(key) && (L.cupp !== upp || L.cfloor !== floor)) cluster(key);
    L.g.style.display = on ? "" : "none";
  }

  function resetContent() {
    closeCard();
    for (const k of LAYERS) {
      const L = layers[k];
      L.g.replaceChildren();
      L.pins = [];
      L.cpins = [];
      L.cg = null;
      L.built = false;
    }
  }

  // setContent ставит метки карты вместе со строкой поиска лута — так каждый слой
  // строится один раз. Возвращает число найденного (лут, контейнеры, замки).
  function setContent(c, q) {
    data = c;
    lootQ = parseQuery(q);
    matchLoot();
    resetContent();
    for (const k of LAYERS) {
      if (k === "loot_hit") show(k, !!lootHits);
      else if (layers[k].on) build(k);
    }
    return hitCount();
  }

  // setLootQuery подсвечивает точки лута с подходящим предметом, контейнеры и замки и
  // возвращает их число. Найденное видно, даже когда его слой выключен.
  function setLootQuery(q) {
    lootQ = parseQuery(q);
    matchLoot();
    for (const k of ["loot_all", "loot_hit"]) {
      const L = layers[k];
      L.g.replaceChildren();
      L.pins = [];
      L.cpins = [];
      L.cg = null;
      L.built = false;
    }
    show("loot_hit", !!lootHits);
    if (layers.loot_all.on) build("loot_all");
    return hitCount();
  }

  // cleanSVG оставляет от карты только рисунок. Карту скачивает scripts/seed при сборке
  // с чужого сервера, а у страницы есть доступ к методам приложения, поэтому белый список: только элементы
  // SVG (HTML внутри — iframe, img — исполняется и без foreignObject), без скриптов и
  // анимаций (set/animate умеют подменить href на javascript:), без обработчиков on*,
  // ссылки — только на свои элементы и встроенные картинки.
  const SVG_NS = "http://www.w3.org/2000/svg";
  const BANNED = /^(script|foreignobject|set|animate|animatemotion|animatetransform|discard)$/i;
  function cleanSVG(root) {
    for (const n of [...root.querySelectorAll("*")]) {
      if (n.namespaceURI !== SVG_NS || BANNED.test(n.localName)) n.remove();
    }
    for (const n of [root, ...root.querySelectorAll("*")]) {
      for (const a of [...n.attributes]) {
        const name = a.name.toLowerCase(), v = a.value.trim().toLowerCase();
        const href = name === "href" || name.endsWith(":href");
        const ok = !href || v.startsWith("#") || (n.localName === "image" && /^data:image\/(png|jpeg|webp);/.test(v));
        if (name.startsWith("on") || !ok) n.removeAttributeNode(a);
      }
    }
  }

  function setSVG(text, rect, layers) {
    base.replaceChildren();
    floorGroups = [];
    R = { x: rect.X, y: rect.Y, w: rect.W, h: rect.H };
    const doc = new DOMParser().parseFromString(text, "image/svg+xml");
    const root = doc.documentElement;
    if (root && root.nodeName === "svg") {
      cleanSVG(root);
      const inner = document.importNode(root, true);
      // растянуть на прямоугольник; preserveAspectRatio по умолчанию — как в Leaflet
      inner.setAttribute("x", R.x);
      inner.setAttribute("y", R.y);
      inner.setAttribute("width", R.w);
      inner.setAttribute("height", R.h);
      base.appendChild(inner);
      floorGroups = [...inner.children].filter((n) => n.nodeName === "g" && n.id);
    }
    baseLayer = layers.length ? layers[0].ID : "";
    layerIds = new Set(layers.slice(1).map((l) => l.ID));
    floorOrder = layers.map((l, i) => ({ id: l.ID, lv: i ? floorLevel(l.Name) : 1, i }))
      .sort((a, b) => a.lv - b.lv || a.i - b.i).map((x) => x.id);
    floor = baseLayer;
    markers = [];
    data = null;
    resetContent();
    vb = null; // вписать после первой отрисовки меток
  }

  // floorLevel — высота этажа для порядка снизу вверх (базовый слой — 1). В данных этажи
  // идут не по высоте, а диапазоны высот у разных зданий одного этажа разные — порядок по имени.
  function floorLevel(name) {
    const m = /^(\d+)(st|nd|rd|th) Floor$/i.exec(name || "");
    if (m) return +m[1];
    if (/^(Underground|Basement|Bunkers|Tunnels|Garage|Technical)/i.test(name || "")) return 0;
    return 1.5;
  }

  function clear() {
    base.replaceChildren();
    for (const k in groups) groups[k].replaceChildren();
    data = null;
    resetContent();
    R = null;
    vb = null;
    markers = [];
    pins = [];
    floorGroups = [];
    floorOrder = [];
  }

  function extractGroup(m, side) {
    if (m.Faction === "shared") return "extract_shared";
    return m.Faction === side ? "extract_own" : "extract_other";
  }

  // подпись справа от значка на тёмной плашке — читается поверх любой подложки;
  // класс lbl — для флажка «Подписи»
  function label(g, text, color) {
    const lg = el("g", { class: "lbl" }, g);
    const plate = el("rect", { x: 10, y: -9.5, height: 19, rx: 4, fill: "rgba(12,11,9,.8)" }, lg);
    const t = el("text", { x: 15, y: 4.5, fill: color, "font-size": 12.5, "font-weight": 500 }, lg);
    t.textContent = text;
    let w = 0;
    try { w = t.getComputedTextLength(); } catch (e) { /* без раскладки — оценим по длине */ }
    plate.setAttribute("width", (w || text.length * 7) + 10);
  }

  function render(view, side) {
    // ширину подписей можно измерить только у видимого текста: скрытые фильтром
    // группы и подписи на время отрисовки показываются
    const shown = {}, noLabels = svg.classList.contains("no-labels");
    for (const k in groups) {
      shown[k] = groups[k].style.display;
      groups[k].style.display = "";
    }
    svg.classList.remove("no-labels");
    drawMarkers(view, side);
    for (const k in groups) groups[k].style.display = shown[k];
    svg.classList.toggle("no-labels", noLabels);
  }

  function drawMarkers(view, side) {
    for (const k in groups) groups[k].replaceChildren();
    pins = [];
    tip.style.display = "none"; // элемент под курсором пересоздан — mouseleave уже не придёт
    if (!R) return;
    markers = view.Markers.filter((m) => m.Kind !== "shot");

    for (const m of view.Markers) {
      if (m.Kind === "extract") {
        const color = css("--" + m.Faction) || css("--shared");
        const g = pin(groups[extractGroup(m, side)], m.PX, m.PY, m.Layer), key = "x:" + m.Name, zone = zoneOf(m, color, key);
        el("rect", { x: -6, y: -6, width: 12, height: 12, rx: 2.5, fill: color, stroke: "#fff", "stroke-width": 1.5 }, g);
        label(g, m.Name + (m.Paid ? " ₽" : "") + (m.Switch ? " ⚡" : ""), color);
        const cond = [];
        if (m.Paid) cond.push(m.Need ? "плата: " + m.Need : "платный: нужны деньги или предмет");
        if (m.Switch) cond.push("сначала включить рубильник");
        hover(g, [m.Name, FACTION[m.Faction] || m.Faction, ...cond].join(" · "), zone);
        onClick.set(g, () => openCard(exitCard(m, key, zone), m.PX, m.PY));
      } else if (m.Kind === "transit") {
        const color = css("--transit");
        const g = pin(groups.transit, m.PX, m.PY, m.Layer), key = "t:" + m.Name, zone = zoneOf(m, color, key);
        el("circle", { r: 6.5, fill: color, stroke: "#fff", "stroke-width": 1.5 }, g);
        label(g, m.Name, color);
        hover(g, m.Name + " · переход на другую карту", zone);
        onClick.set(g, () => openCard({ key, zone, title: m.Name, sub: "Переход на другую карту" }, m.PX, m.PY));
      }
    }

    // последний скрин — «где я сейчас»; линия и прежние точки — «пройденный путь»
    const shots = view.Markers.filter((m) => m.Kind === "shot");
    const red = css("--route");
    if (shots.length > 1) {
      el("polyline", {
        points: shots.map((s) => s.PX + "," + s.PY).join(" "),
        fill: "none", stroke: red, "stroke-width": 2.5, "stroke-dasharray": "7 5", opacity: 0.8,
        "vector-effect": "non-scaling-stroke",
      }, groups.trail);
    }
    shots.forEach((s, i) => {
      const last = i === shots.length - 1;
      const g = pin(last ? groups.position : groups.trail, s.PX, s.PY, s.Layer);
      // конус взгляда: DirX/DirY — единичный вектор в координатах карты
      const ang = Math.atan2(s.DirY, s.DirX), len = last ? 46 : 30, half = 0.45;
      if (s.DirX || s.DirY) {
        el("path", {
          d: `M0,0 L${len * Math.cos(ang - half)},${len * Math.sin(ang - half)} ` +
             `A${len},${len} 0 0 1 ${len * Math.cos(ang + half)},${len * Math.sin(ang + half)} Z`,
          fill: red, opacity: last ? 0.55 : 0.25,
        }, g);
      }
      el("circle", { r: last ? 7 : 4.5, fill: last ? red : "#fff", stroke: last ? "#fff" : red, "stroke-width": 2 }, g);
      if (last) {
        hover(g, `Я здесь · скрин в ${s.Time}`);
        onClick.set(g, () => openCard({ title: "Я здесь", sub: "Скрин в " + s.Time }, s.PX, s.PY));
        return;
      }
      const t = el("text", { y: -9, fill: "#fff", "font-size": 11, "text-anchor": "middle", "paint-order": "stroke", stroke: "#000", "stroke-width": 3 }, g);
      t.textContent = i + 1;
      hover(g, `#${i + 1} · ${s.Time}`);
      onClick.set(g, () => openCard({ title: `Точка пути №${i + 1}`, sub: "Скрин в " + s.Time }, s.PX, s.PY));
    });
    for (const k in groups) dimNodes(groups[k]); // слои меток перерисовка вида не трогает
    if (!vb) fit();
  }

  // zoneOf — скрытый контур зоны выхода или перехода; у открытой карточки — сразу виден
  function zoneOf(m, color, key) {
    if (m.Outline.length < 3) return null;
    const zone = el("polygon", {
      points: m.Outline.map((p) => p[0] + "," + p[1]).join(" "), "data-layer": m.Layer, style: "display:none",
      fill: color, "fill-opacity": 0.18, stroke: color, "stroke-width": 2, "vector-effect": "non-scaling-stroke",
    }, groups.zone);
    if (key === cardKey) {
      zone.style.display = "";
      pinnedZone = zone;
    }
    return zone;
  }

  function exitCard(m, key, zone) {
    const lines = [];
    if (m.Paid) lines.push("Плата: " + (m.Need || "деньги или предмет"));
    const sw = data ? m.Switches.map((id) => data.Switches.find((s) => s.ID === id)).filter(Boolean) : [];
    if (m.Switch && !sw.length) lines.push("Сначала включить рубильник");
    return { key, zone, title: m.Name, sub: EXIT[m.Faction] || "Выход", lines, links: [{ head: "Сначала включить", items: sw.map(at) }] };
  }

  function setFloor(id) {
    floor = id;
    applyFloor();
  }

  // Как в tarkov.dev: базовый слой виден всегда, вместе с ним — группы с data-keep-with-group
  // (первые этажи зданий, лестницы) и всё, что не является отдельным этажом. Выбран этаж со
  // своей группой — база приглушается, остальные этажи скрыты. У этажа только из тайлов своей
  // группы нет — показываем базу как есть.
  function applyFloor() {
    const isBase = (g) => g.dataset.keepWithGroup === baseLayer || !layerIds.has(g.id);
    const own = floorGroups.some((g) => g.id === floor && !isBase(g));
    floorGroups.forEach((g) => {
      const base = isBase(g);
      g.classList.toggle("floor-hidden", !base && !(own && g.id === floor));
      g.classList.toggle("floor-dim", base && own);
    });
    dimNodes(overlay);
    for (const k of CLUSTERED) if (layers[k].on && layers[k].built) cluster(k);
  }

  // метки чужого этажа приглушены; названия мест не приглушаются, а подпись не базового
  // этажа видна только на своём — иначе магазины Развязки засыпали бы ТЦ на виде с земли
  // Метки «Лут и места» — только своего этажа (подписи земли — всегда); выходы и маршрут
  // с чужого этажа приглушены: выход нужен, на каком этаже ни стой.
  function dimNodes(root) {
    root.querySelectorAll("[data-layer]").forEach((n) => {
      const own = n.dataset.layer === floor;
      if (n.classList.contains("place")) n.style.display = own || n.dataset.layer === baseLayer ? "" : "none";
      else if (content.contains(n)) n.style.display = own ? "" : "none";
      else n.style.opacity = own ? 1 : 0.35;
    });
  }

  // lootHitsElsewhere — сколько найденного поиском на других этажах (оно скрыто).
  function lootHitsElsewhere() {
    let n = 0;
    if (!lootHits || !data) return 0;
    for (const i of lootHits) if (data.Loot[i].Layer !== floor) n++;
    for (const i of boxHits) if (data.Containers[i].Layer !== floor) n++;
    for (const i of lockHits) if (data.Locks[i].Layer !== floor) n++;
    return n;
  }

  function setFilter(key, on) {
    if (key === "labels") svg.classList.toggle("no-labels", !on);
    else if (layers[key]) show(key, on);
    else if (groups[key]) groups[key].style.display = on ? "" : "none";
  }

  // --- масштаб и сдвиг через viewBox ---
  const MAX_ZOOM = 8; // во сколько раз можно приблизить относительно всей карты
  let moved = false;  // пользователь двигал карту — не вписывать её заново при смене размера
  const sized = () => R && svg.clientWidth && svg.clientHeight;

  function draw() {
    if (!vb) return;
    vb = clampView(vb);
    svg.setAttribute("viewBox", `${vb.x} ${vb.y} ${vb.w} ${vb.h}`);
    const u = svg.clientWidth ? vb.w / svg.clientWidth : 0;
    if (u && u !== upp) {
      upp = u;
      pins.forEach(place);
      flashes.forEach(place);
      for (const k of LAYERS) {
        const L = layers[k];
        if (L.on && L.built) {
          L.pins.forEach(place);
          L.cpins.forEach(place);
          L.upp = upp;
        }
      }
      reclusterSoon();
    }
    positionCard();
  }

  // карта вместе с метками за её краем (переходы и выходы бывают снаружи) и полями 3 %
  function contentBox() {
    let x0 = R.x, y0 = R.y, x1 = R.x + R.w, y1 = R.y + R.h;
    for (const m of markers) {
      x0 = Math.min(x0, m.PX); y0 = Math.min(y0, m.PY);
      x1 = Math.max(x1, m.PX); y1 = Math.max(y1, m.PY);
    }
    const pad = 0.03 * Math.max(x1 - x0, y1 - y0);
    return { x0: x0 - pad, y0: y0 - pad, x1: x1 + pad, y1: y1 + pad };
  }

  // вид на всю карту при текущих пропорциях окна
  function fullView() {
    const b = contentBox(), w = b.x1 - b.x0, h = b.y1 - b.y0, ar = svg.clientWidth / svg.clientHeight;
    return w / h > ar
      ? { x: b.x0, w, h: w / ar, y: b.y0 - (w / ar - h) / 2 }
      : { y: b.y0, h, w: h * ar, x: b.x0 - (h * ar - w) / 2 };
  }

  // Отдалить — не дальше всей карты, приблизить — не больше MAX_ZOOM раз. Сдвиг: карта
  // меньше вида остаётся в нём целиком, больше вида — вид не выходит за её край.
  function clampView(v) {
    if (!sized()) return v;
    const full = fullView(), b = contentBox();
    let { x, y, w, h } = v;
    const nw = Math.min(full.w, Math.max(full.w / MAX_ZOOM, w));
    if (nw !== w) {
      const cx = x + w / 2, cy = y + h / 2;
      h = (h * nw) / w;
      w = nw;
      x = cx - w / 2;
      y = cy - h / 2;
    }
    const fix = (p, size, lo, hi) =>
      size >= hi - lo ? Math.min(lo, Math.max(hi - size, p)) : Math.min(hi - size, Math.max(lo, p));
    return { x: fix(x, w, b.x0, b.x1), y: fix(y, h, b.y0, b.y1), w, h };
  }

  function fit() {
    moved = false;
    if (!sized()) return;
    vb = fullView();
    draw();
  }

  function centerOn(px, py) {
    if (!vb) return;
    moved = true;
    vb = { ...vb, x: px - vb.w / 2, y: py - vb.h / 2 };
    draw();
  }

  // keepInView центрирует на точке, только если она вышла из средней части вида (70 %):
  // при приближении карта едет за игроком, а вид на всю карту не сдвигается — видно и так
  function keepInView(px, py) {
    if (!vb || !sized() || vb.w >= fullView().w * 0.999) return;
    const mx = vb.w * 0.15, my = vb.h * 0.15;
    if (px < vb.x + mx || px > vb.x + vb.w - mx || py < vb.y + my || py > vb.y + vb.h - my) centerOn(px, py);
  }

  // zoomAt меняет ширину вида в k раз (в пределах масштаба), точка (mx, my) остаётся на месте
  function zoomAt(mx, my, k, center) {
    const full = fullView(), w = Math.min(full.w, Math.max(full.w / MAX_ZOOM, vb.w * k));
    k = w / vb.w;
    if (k === 1 && !center) return;
    moved = true;
    vb = center
      ? { x: mx - (vb.w * k) / 2, y: my - (vb.h * k) / 2, w: vb.w * k, h: vb.h * k }
      : { x: mx - (mx - vb.x) * k, y: my - (my - vb.y) * k, w: vb.w * k, h: vb.h * k };
    draw();
  }

  // Ctrl + колесо — этаж выше или ниже; тачпад шлёт десятки событий — не чаще раза в 200 мс
  let floorWheelAt = 0;
  function wheelFloor(e) {
    const i = floorOrder.indexOf(floor), j = Math.min(floorOrder.length - 1, Math.max(0, i + (e.deltaY < 0 ? 1 : -1)));
    if (i < 0 || j === i || e.timeStamp - floorWheelAt < 200) return;
    floorWheelAt = e.timeStamp;
    if (onFloorCb) onFloorCb(floorOrder[j]);
    else setFloor(floorOrder[j]);
  }

  svg.addEventListener("wheel", (e) => {
    e.preventDefault(); // и масштаб страницы по Ctrl + колесо в WebView2
    if (!vb || !sized()) return;
    // щипок на тачпаде приходит колесом с Ctrl и мелким шагом — это масштаб, а не этаж
    if (e.ctrlKey && Math.abs(e.deltaY) >= 40) {
      wheelFloor(e);
      return;
    }
    // множитель сразу в пределах масштаба, чтобы точка под курсором осталась на месте
    zoomAt(vb.x + (vb.w * e.offsetX) / svg.clientWidth, vb.y + (vb.h * e.offsetY) / svg.clientHeight, e.deltaY > 0 ? 1.2 : 1 / 1.2);
  }, { passive: false });

  let drag = null;
  svg.addEventListener("mousedown", (e) => {
    if (!vb || e.button !== 0) return;
    drag = { x: e.clientX, y: e.clientY, vb: { ...vb } };
    dragged = false;
    svg.classList.add("drag");
  });
  addEventListener("mousemove", (e) => {
    if (!drag) return;
    // дрожь руки при щелчке — не сдвиг: иначе клик по метке не открыл бы карточку
    if (!dragged && Math.hypot(e.clientX - drag.x, e.clientY - drag.y) < 4) return;
    dragged = true;
    moved = true;
    const s = vb.w / svg.clientWidth;
    vb = { ...drag.vb, x: drag.vb.x - (e.clientX - drag.x) * s, y: drag.vb.y - (e.clientY - drag.y) * s };
    draw();
  });
  addEventListener("mouseup", () => {
    drag = null;
    svg.classList.remove("drag");
  });

  // смена размера области (окно, свёрнутая лента): не трогали — вписать заново,
  // иначе сохранить центр и масштаб в пикселях
  let size = { w: 0, h: 0 };
  new ResizeObserver(() => {
    const w = svg.clientWidth, h = svg.clientHeight;
    if (!w || !h) return;
    if (vb && moved && size.w) {
      const k = vb.w / size.w, cx = vb.x + vb.w / 2, cy = vb.y + vb.h / 2;
      vb = { x: cx - (w * k) / 2, y: cy - (h * k) / 2, w: w * k, h: h * k };
      draw();
    } else {
      fit();
    }
    size = { w, h };
  }).observe(svg);

  const onFloor = (fn) => (onFloorCb = fn);

  return { setSVG, clear, render, setContent, setLootQuery, lootHitsElsewhere, setFloor, onFloor, setFilter, centerOn, keepInView, fit };
})();
