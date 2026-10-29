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
  // порядок — снизу вверх: путь поверх выходов, текущее место поверх всего
  const groups = {};
  for (const k of ["transit", "extract_other", "extract_shared", "extract_own", "trail", "position"]) groups[k] = el("g", {}, overlay);

  let R = null;          // прямоугольник карты в координатах вида
  let vb = null;         // текущий viewBox
  let floorGroups = [];  // этажи внутри SVG
  let baseLayer = "", floor = "";
  let layerIds = new Set(); // этажи кроме базового
  let markers = [];      // выходы и переходы последней отрисовки — для вписывания
  let pins = [];         // группы меток в пикселях экрана: {g, x, y}
  let upp = 1;           // единиц карты на пиксель экрана

  const FACTION = { pmc: "ЧВК", scav: "Дикие", shared: "Совместный" };

  function hover(node, text) {
    node.addEventListener("mousemove", (ev) => {
      tip.style.display = "block";
      tip.textContent = text;
      tip.style.left = ev.clientX + 12 + "px";
      tip.style.top = ev.clientY + 12 + "px";
    });
    node.addEventListener("mouseleave", () => (tip.style.display = "none"));
  }

  // pin — группа в точке карты, её содержимое задаётся в пикселях экрана
  const place = (p) => p.g.setAttribute("transform", `translate(${p.x},${p.y}) scale(${upp})`);
  function pin(parent, x, y, layer) {
    const p = { g: el("g", { "data-layer": layer }, parent), x, y };
    pins.push(p);
    place(p);
    return p.g;
  }

  function setSVG(text, rect, layers) {
    base.replaceChildren();
    floorGroups = [];
    R = { x: rect.X, y: rect.Y, w: rect.W, h: rect.H };
    const doc = new DOMParser().parseFromString(text, "image/svg+xml");
    const root = doc.documentElement;
    if (root && root.nodeName === "svg") {
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
    floor = baseLayer;
    markers = [];
    vb = null; // вписать после первой отрисовки меток
  }

  function clear() {
    base.replaceChildren();
    for (const k in groups) groups[k].replaceChildren();
    R = null;
    vb = null;
    markers = [];
    pins = [];
    floorGroups = [];
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
        const g = pin(groups[extractGroup(m, side)], m.PX, m.PY, m.Layer);
        el("rect", { x: -6, y: -6, width: 12, height: 12, rx: 2.5, fill: color, stroke: "#fff", "stroke-width": 1.5 }, g);
        label(g, m.Name + (m.Paid ? " ₽" : "") + (m.Switch ? " ⚡" : ""), color);
        const cond = [];
        if (m.Paid) cond.push("платный: нужны деньги или предмет");
        if (m.Switch) cond.push("сначала включить рубильник");
        hover(g, [m.Name, FACTION[m.Faction] || m.Faction, ...cond].join(" · "));
      } else if (m.Kind === "transit") {
        const color = css("--transit");
        const g = pin(groups.transit, m.PX, m.PY, m.Layer);
        el("circle", { r: 6.5, fill: color, stroke: "#fff", "stroke-width": 1.5 }, g);
        label(g, m.Name, color);
        hover(g, m.Name + " · переход на другую карту");
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
        return;
      }
      const t = el("text", { y: -9, fill: "#fff", "font-size": 11, "text-anchor": "middle", "paint-order": "stroke", stroke: "#000", "stroke-width": 3 }, g);
      t.textContent = i + 1;
      hover(g, `#${i + 1} · ${s.Time}`);
    });
    applyFloor();
    if (!vb) fit();
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
    overlay.querySelectorAll("[data-layer]").forEach((n) => (n.style.opacity = n.dataset.layer === floor ? 1 : 0.35));
  }

  function setFilter(key, on) {
    if (key === "labels") svg.classList.toggle("no-labels", !on);
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
    }
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

  svg.addEventListener("wheel", (e) => {
    e.preventDefault();
    if (!vb || !sized()) return;
    // множитель сразу в пределах масштаба, чтобы точка под курсором осталась на месте
    const full = fullView(), w = Math.min(full.w, Math.max(full.w / MAX_ZOOM, vb.w * (e.deltaY > 0 ? 1.2 : 1 / 1.2)));
    const k = w / vb.w;
    if (k === 1) return;
    moved = true;
    const mx = vb.x + (vb.w * e.offsetX) / svg.clientWidth, my = vb.y + (vb.h * e.offsetY) / svg.clientHeight;
    vb = { x: mx - (mx - vb.x) * k, y: my - (my - vb.y) * k, w: vb.w * k, h: vb.h * k };
    draw();
  }, { passive: false });

  let drag = null;
  svg.addEventListener("mousedown", (e) => {
    if (!vb || e.button !== 0) return;
    drag = { x: e.clientX, y: e.clientY, vb: { ...vb } };
    svg.classList.add("drag");
  });
  addEventListener("mousemove", (e) => {
    if (!drag) return;
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

  return { setSVG, clear, render, setFloor, setFilter, centerOn, keepInView, fit };
})();
