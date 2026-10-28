// Отрисовка карты: SVG-подложка, маркеры по готовым координатам (PX/PY), этажи, масштаб и сдвиг.
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
  // порядок — снизу вверх: маршрут поверх выходов
  const groups = {};
  for (const k of ["transit", "extract_other", "extract_shared", "extract_own", "route"]) groups[k] = el("g", {}, overlay);

  let R = null;          // прямоугольник карты в координатах вида
  let vb = null;         // текущий viewBox
  let floorGroups = [];  // этажи внутри SVG
  let baseLayer = "", floor = "";
  let markers = [];      // выходы и переходы последней отрисовки — для вписывания

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
    floorGroups = [];
  }

  function extractGroup(m, side) {
    if (m.Faction === "shared") return "extract_shared";
    return m.Faction === side ? "extract_own" : "extract_other";
  }

  function label(parent, x, y, text, color, U) {
    const t = el("text", { x, y, fill: color, "font-size": 3.6 * U, "paint-order": "stroke", stroke: "#000", "stroke-width": 0.8 * U }, parent);
    t.textContent = text;
  }

  function render(view, side) {
    for (const k in groups) groups[k].replaceChildren();
    if (!R) return;
    const U = R.w / 300; // базовая единица размера маркеров
    markers = view.Markers.filter((m) => m.Kind !== "shot");

    for (const m of view.Markers) {
      if (m.Kind === "extract") {
        const color = css("--" + m.Faction) || css("--shared");
        const g = el("g", { "data-layer": m.Layer }, groups[extractGroup(m, side)]);
        el("rect", { x: m.PX - 2.2 * U, y: m.PY - 2.2 * U, width: 4.4 * U, height: 4.4 * U, rx: U, fill: color, stroke: "#000", "stroke-width": 0.4 * U }, g);
        label(g, m.PX + 3 * U, m.PY + 1.3 * U, m.Name + (m.Paid ? " ₽" : "") + (m.Switch ? " ⚡" : ""), color, U);
        const cond = [];
        if (m.Paid) cond.push("платный: нужны деньги или предмет");
        if (m.Switch) cond.push("сначала включить рубильник");
        hover(g, [m.Name, FACTION[m.Faction] || m.Faction, ...cond].join(" · "));
      } else if (m.Kind === "transit") {
        const color = css("--transit");
        const g = el("g", { "data-layer": m.Layer }, groups.transit);
        el("circle", { cx: m.PX, cy: m.PY, r: 2.4 * U, fill: color, stroke: "#000", "stroke-width": 0.4 * U }, g);
        label(g, m.PX + 3 * U, m.PY + 1.3 * U, m.Name, color, U);
        hover(g, m.Name + " · переход на другую карту");
      }
    }

    const shots = view.Markers.filter((m) => m.Kind === "shot");
    const red = css("--route");
    if (shots.length > 1) {
      el("polyline", {
        points: shots.map((s) => s.PX + "," + s.PY).join(" "),
        fill: "none", stroke: red, "stroke-width": 0.7 * U, "stroke-dasharray": `${2 * U} ${1.2 * U}`, opacity: 0.8,
      }, groups.route);
    }
    shots.forEach((s, i) => {
      const last = i === shots.length - 1;
      const g = el("g", { "data-layer": s.Layer }, groups.route);
      // конус взгляда: DirX/DirY — единичный вектор в координатах карты
      const ang = Math.atan2(s.DirY, s.DirX), len = 9 * U, half = 0.45;
      if (s.DirX || s.DirY) {
        el("path", {
          d: `M${s.PX},${s.PY} L${s.PX + len * Math.cos(ang - half)},${s.PY + len * Math.sin(ang - half)} ` +
             `A${len},${len} 0 0 1 ${s.PX + len * Math.cos(ang + half)},${s.PY + len * Math.sin(ang + half)} Z`,
          fill: red, opacity: last ? 0.55 : 0.25,
        }, g);
      }
      el("circle", { cx: s.PX, cy: s.PY, r: (last ? 2.2 : 1.6) * U, fill: last ? red : "#fff", stroke: red, "stroke-width": 0.6 * U }, g);
      const t = el("text", { x: s.PX, y: s.PY - 2.6 * U, fill: "#fff", "font-size": 3 * U, "text-anchor": "middle", "paint-order": "stroke", stroke: "#000", "stroke-width": 0.7 * U }, g);
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

  function applyFloor() {
    // этажа без своей группы в SVG (только тайлы) нет — показываем базовый слой
    let gi = floorGroups.findIndex((g) => g.id === floor);
    if (gi < 0) gi = floorGroups.findIndex((g) => g.id === baseLayer);
    floorGroups.forEach((g, i) => {
      g.classList.toggle("floor-hidden", gi >= 0 && i > gi);
      g.classList.toggle("floor-dim", gi >= 0 && i < gi);
    });
    overlay.querySelectorAll("[data-layer]").forEach((n) => (n.style.opacity = n.dataset.layer === floor ? 1 : 0.35));
  }

  function setFilter(key, on) {
    if (groups[key]) groups[key].style.display = on ? "" : "none";
  }

  // --- масштаб и сдвиг через viewBox ---
  let moved = false; // пользователь двигал карту — не вписывать её заново при смене размера
  const draw = () => vb && svg.setAttribute("viewBox", `${vb.x} ${vb.y} ${vb.w} ${vb.h}`);

  // вписать карту целиком вместе с метками за её краем (переходы и выходы бывают снаружи)
  function fit() {
    moved = false;
    if (!R || !svg.clientWidth || !svg.clientHeight) return;
    let x0 = R.x, y0 = R.y, x1 = R.x + R.w, y1 = R.y + R.h;
    for (const m of markers) {
      x0 = Math.min(x0, m.PX); y0 = Math.min(y0, m.PY);
      x1 = Math.max(x1, m.PX); y1 = Math.max(y1, m.PY);
    }
    const pad = 0.03 * Math.max(x1 - x0, y1 - y0);
    x0 -= pad; y0 -= pad; x1 += pad; y1 += pad;
    const w = x1 - x0, h = y1 - y0, ar = svg.clientWidth / svg.clientHeight;
    vb = w / h > ar
      ? { x: x0, w, h: w / ar, y: y0 - (w / ar - h) / 2 }
      : { y: y0, h, w: h * ar, x: x0 - (h * ar - w) / 2 };
    draw();
  }

  function centerOn(px, py) {
    if (!vb) return;
    moved = true;
    vb = { ...vb, x: px - vb.w / 2, y: py - vb.h / 2 };
    draw();
  }

  svg.addEventListener("wheel", (e) => {
    e.preventDefault();
    if (!vb) return;
    moved = true;
    const k = e.deltaY > 0 ? 1.2 : 1 / 1.2;
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

  return { setSVG, clear, render, setFloor, setFilter, centerOn, fit };
})();
