package report

// htmlTemplate 是报告页面的完整 HTML 模板（内嵌 CSS/JS，无外部依赖）。
// 渲染时用 JSON 数据替换 /*__REPORT_JSON__*/ 标记。
// 注意：模板内不得出现反引号与 ${}（Go raw string / 避免模板字符串）。
const htmlTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>tdx2db 数据完整性报告</title>
<style>
  * { box-sizing: border-box; }
  html, body { height: 100%; margin: 0; }
  body {
    display: flex; flex-direction: column;
    background: #14171c; color: #d8dee6;
    font-family: -apple-system, "Segoe UI", "Microsoft YaHei", sans-serif;
  }
  header { padding: 14px 20px 10px; border-bottom: 1px solid #262c35; }
  h1 { font-size: 18px; margin: 0 0 4px; }
  #meta { color: #8b95a3; font-size: 12px; margin-bottom: 8px; }
  .warn {
    background: #3a2f12; color: #e3b341; border: 1px solid #5a4a1e;
    padding: 6px 10px; border-radius: 4px; margin: 4px 0; font-size: 12px;
  }
  #summary { display: flex; flex-wrap: wrap; gap: 8px; padding: 10px 20px 4px; align-items: flex-start; }
  #cards { display: flex; flex-wrap: wrap; gap: 8px; }
  .card {
    background: #1c2128; border: 1px solid #2a313b; border-radius: 6px;
    padding: 6px 14px; text-align: center;
  }
  .card .v { font-size: 16px; font-weight: 600; }
  .card .k { font-size: 11px; color: #8b95a3; }
  #mbd {
    background: #1c2128; border: 1px solid #2a313b; border-radius: 6px;
    padding: 6px 12px; font-size: 12px; max-height: 220px; overflow: auto;
  }
  #mbd h3 { margin: 2px 0 6px; font-size: 12px; color: #8b95a3; }
  #mbd table { border-collapse: collapse; }
  #mbd td, #mbd th { padding: 2px 10px; border-bottom: 1px solid #262c35; text-align: right; }
  #mbd td:first-child, #mbd th:first-child { text-align: left; }
  #absent { padding: 0 20px; font-size: 12px; color: #8b95a3; }
  #absent summary { cursor: pointer; }
  #absent .absent { word-break: break-all; max-height: 120px; overflow: auto; margin-top: 4px; }
  #toolbar {
    display: flex; align-items: center; gap: 14px; flex-wrap: wrap;
    padding: 8px 20px; border-bottom: 1px solid #262c35; font-size: 13px;
  }
  .tab {
    background: #1c2128; color: #8b95a3; border: 1px solid #2a313b;
    padding: 5px 16px; border-radius: 4px; cursor: pointer; font-size: 13px;
  }
  .tab.on { background: #2a313b; color: #fff; border-color: #3a4653; }
  #search {
    background: #1c2128; color: #d8dee6; border: 1px solid #2a313b;
    border-radius: 4px; padding: 5px 10px; width: 180px; font-size: 13px;
  }
  #zoomBtn {
    background: #1c2128; color: #8b95a3; border: 1px solid #2a313b;
    border-radius: 4px; padding: 5px 12px; cursor: pointer; font-size: 13px;
  }
  #legend { display: flex; gap: 16px; padding: 6px 20px; font-size: 12px; color: #8b95a3; }
  .sw { display: inline-block; width: 12px; height: 12px; border-radius: 2px; margin-right: 5px; vertical-align: -2px; }
  #wrap { flex: 1; overflow: auto; position: relative; margin: 0 20px 8px; border: 1px solid #262c35; border-radius: 6px; background: #14171c; }
  #spacer { position: relative; }
  #cv { position: absolute; left: 0; top: 0; display: block; }
  #detail {
    margin: 0 20px 8px; padding: 8px 14px; background: #1c2128;
    border: 1px solid #2a313b; border-radius: 6px; font-size: 13px;
    max-height: 200px; overflow: auto;
  }
  #detail h3 { margin: 2px 0 6px; font-size: 14px; }
  #detail ul { margin: 4px 0; padding-left: 22px; }
  #detail .empty { color: #8b95a3; }
  #detail .ok { color: #2f9e63; }
  #detail .miss { color: #d64550; }
  #detail .part { color: #e3b341; }
  #tip {
    position: fixed; display: none; z-index: 10; pointer-events: none;
    background: #000c; border: 1px solid #3a4653; border-radius: 4px;
    padding: 6px 10px; font-size: 12px; color: #d8dee6; white-space: nowrap;
  }
  footer { padding: 6px 20px 10px; font-size: 11px; color: #5c6672; }
</style>
</head>
<body>
<header>
  <h1>tdx2db 数据完整性报告</h1>
  <div id="meta"></div>
  <div id="warnEl"></div>
</header>
<div id="summary">
  <div id="cards"></div>
  <div id="mbd"></div>
  <div id="absent"></div>
</div>
<div id="toolbar">
  <button class="tab on" data-kind="daily">日线</button>
  <button class="tab" data-kind="min">1分钟</button>
  <label><input type="checkbox" id="onlyMiss" checked> 只显示有缺口的品种</label>
  <input id="search" placeholder="搜索代码 / 名称">
  <button id="zoomBtn">重置缩放</button>
  <span id="zoomInfo" style="color:#8b95a3;font-size:12px">拖拽表头缩放区间，双击表头重置</span>
</div>
<div id="legend">
  <span><span class="sw" style="background:#2f9e63"></span>有数据（分时：完整 ≥240 分钟）</span>
  <span><span class="sw" style="background:#e3b341"></span>分时部分（1~239 分钟）</span>
  <span><span class="sw" style="background:#d64550"></span>缺失</span>
  <span><span class="sw" style="background:#343a44"></span>窗口外（上市前）</span>
</div>
<div id="wrap"><div id="spacer"></div><canvas id="cv"></canvas></div>
<div id="detail"><div class="empty">点击左侧行查看缺失区段明细</div></div>
<div id="tip"></div>
<footer>注：停牌期间无行情 bar，个别品种孤立红色的日子可能是停牌而非数据缺失。市场级缺失日（多数品种同缺）才是下载/更新问题的特征。</footer>
<script>
'use strict';
var R = /*__REPORT_JSON__*/;
var geom = { labelW: 260, colW: 5, rowH: 18, headH: 64 };
var COLORS = { green: '#2f9e63', yellow: '#e3b341', red: '#d64550', gray: '#343a44' };

var kind = 'daily', onlyMiss = true, search = '', rangeA = null, rangeB = null, selected = -1, drag = null;

var cv = document.getElementById('cv');
var wrap = document.getElementById('wrap');
var tip = document.getElementById('tip');
var ctx = cv.getContext('2d');
var metaEl = document.getElementById('meta');
var warnEl = document.getElementById('warnEl');
var cardsEl = document.getElementById('cards');
var mbdEl = document.getElementById('mbd');
var absentEl = document.getElementById('absent');
var detailEl = document.getElementById('detail');
var onlyMissEl = document.getElementById('onlyMiss');
var searchEl = document.getElementById('search');
var zoomInfoEl = document.getElementById('zoomInfo');

function grid() { return kind === 'daily' ? R.daily : R.min; }

function rowBits(r, i) {
  var b = atob(r.b);
  return (b.charCodeAt(i >> 3) >> (7 - (i & 7))) & 1;
}

function viewRows() {
  var g = grid(), q = search.trim().toLowerCase(), out = [];
  for (var i = 0; i < g.rows.length; i++) {
    var r = g.rows[i];
    if (onlyMiss && (r.m || []).length === 0) continue;
    if (q && r.s.toLowerCase().indexOf(q) < 0 && (r.n || '').toLowerCase().indexOf(q) < 0) continue;
    out.push(r);
  }
  return out;
}

function colRange() {
  var g = grid();
  return { a: rangeA === null ? 0 : rangeA, b: rangeB === null ? g.days.length - 1 : rangeB };
}

function cellColor(g, r, i) {
  if (i < r.f) return COLORS.gray;
  if (rowBits(r, i)) {
    if (g.kind === 'daily') return COLORS.green;
    var pc = r.pc || [];
    for (var k = 0; k < pc.length; k++) if (pc[k].i === i) return COLORS.yellow;
    return COLORS.green;
  }
  return COLORS.red;
}

function cellStatus(g, r, i) {
  if (i < r.f) return '窗口外（上市前）';
  if (g.kind === 'daily') return rowBits(r, i) ? '有数据' : '缺失';
  if (!rowBits(r, i)) return '缺失';
  var pc = r.pc || [];
  for (var k = 0; k < pc.length; k++) if (pc[k].i === i) return '部分完整：' + pc[k].c + ' 分钟';
  return '完整（≥240 分钟）';
}

function pos(e) {
  var rect = cv.getBoundingClientRect();
  return { mx: e.clientX - rect.left + wrap.scrollLeft, my: e.clientY - rect.top + wrap.scrollTop };
}

function draw() {
  var g = grid(), rows = viewRows(), cr = colRange();
  var ncols = cr.b - cr.a + 1;
  var fullW = geom.labelW + ncols * geom.colW;
  var fullH = geom.headH + rows.length * geom.rowH;
  var spacer = document.getElementById('spacer');
  spacer.style.width = fullW + 'px';
  spacer.style.height = fullH + 'px';

  var W = wrap.clientWidth, H = wrap.clientHeight;
  cv.width = W; cv.height = H;
  cv.style.width = W + 'px'; cv.style.height = H + 'px';
  var sx = wrap.scrollLeft, sy = wrap.scrollTop;
  // 关键：canvas 是 absolute 定位，随内容一起滚动；
  // 必须把它钉回视口（top/left 同步滚动偏移），否则滚动后大片区域露出背景。
  cv.style.left = sx + 'px';
  cv.style.top = sy + 'px';

  ctx.fillStyle = '#14171c';
  ctx.fillRect(0, 0, W, H);

  if (rows.length === 0) {
    ctx.fillStyle = '#8b95a3';
    ctx.font = '14px sans-serif';
    ctx.fillText(onlyMiss ? '当前维度全部品种无缺口 🎉' : '没有符合筛选的品种', W / 2 - 120, H / 2);
    return;
  }

  var r0 = Math.max(0, Math.floor(sy / geom.rowH) - 1);
  var r1 = Math.min(rows.length, Math.ceil((sy + H) / geom.rowH) + 1);
  var c0 = Math.max(0, Math.floor(sx / geom.colW) - 1);
  var c1 = Math.min(ncols, Math.ceil((sx + W) / geom.colW) + 1);

  ctx.save();
  ctx.translate(-sx, -sy);
  drawHeader(g, cr.a, c0, c1, ncols);
  for (var ri = r0; ri < r1; ri++) {
    var r = rows[ri];
    var y = geom.headH + ri * geom.rowH;
    ctx.fillStyle = ri % 2 ? '#1a1f26' : '#171c22';
    ctx.fillRect(0, y, fullW, geom.rowH);
    if (ri === selected) {
      ctx.fillStyle = 'rgba(120,160,255,0.10)';
      ctx.fillRect(0, y, geom.labelW, geom.rowH);
    }
    ctx.fillStyle = '#d8dee6';
    ctx.font = '12px monospace';
    ctx.fillText(r.s, 8, y + 13);
    ctx.fillStyle = '#8b95a3';
    ctx.font = '11px sans-serif';
    var nm = r.n || '';
    if (nm.length > 7) nm = nm.slice(0, 7);
    ctx.fillText(nm, 86, y + 13);
    var pct = r.e > 0 ? r.p / r.e : 0;
    ctx.fillStyle = pct >= 0.98 ? COLORS.green : pct >= 0.90 ? COLORS.yellow : COLORS.red;
    ctx.font = 'bold 11px monospace';
    ctx.fillText((pct * 100).toFixed(1) + '%', 180, y + 13);
    for (var ci = c0; ci < c1; ci++) {
      ctx.fillStyle = cellColor(g, r, cr.a + ci);
      ctx.fillRect(geom.labelW + ci * geom.colW, y, geom.colW - 0.4, geom.rowH - 1);
    }
  }
  ctx.restore();

  if (drag) {
    var x0 = geom.labelW + Math.min(drag.start, drag.cur) * geom.colW - sx;
    var x1 = geom.labelW + (Math.max(drag.start, drag.cur) + 1) * geom.colW - sx;
    ctx.fillStyle = 'rgba(90,140,255,0.18)';
    ctx.fillRect(x0, 0, x1 - x0, geom.headH);
  }
}

function drawHeader(g, a, c0, c1, ncols) {
  ctx.fillStyle = '#1c2128';
  ctx.fillRect(0, 0, geom.labelW + ncols * geom.colW, geom.headH);
  ctx.fillStyle = '#8b95a3';
  ctx.font = '11px sans-serif';
  ctx.fillText('股票ID / 名称 / 满足度', 8, 20);
  ctx.font = '10px sans-serif';
  ctx.fillText('拖拽本行缩放，双击重置', 8, 36);
  var lastMonth = '';
  for (var ci = c0; ci < c1; ci++) {
    var di = a + ci;
    var x = geom.labelW + ci * geom.colW;
    var m = g.days[di].slice(0, 7);
    if (m !== lastMonth) {
      lastMonth = m;
      ctx.fillStyle = '#5c6672';
      ctx.fillRect(x, geom.headH - 12, 1, 12);
      ctx.save();
      ctx.translate(x + 3, geom.headH - 16);
      ctx.rotate(-Math.PI / 2);
      ctx.fillStyle = '#8b95a3';
      ctx.font = '10px monospace';
      ctx.fillText(m, 0, 0);
      ctx.restore();
    }
  }
}

function renderSummary() {
  var g = grid();
  var withMiss = 0, pres = 0, exp = 0;
  var rows = g.rows || [], absent = g.absent || [], mbd = g.mbd || [];
  for (var i = 0; i < rows.length; i++) {
    var r = rows[i];
    if ((r.m || []).length) withMiss++;
    pres += r.p; exp += r.e;
  }
  var cov = exp > 0 ? pres / exp : 0;
  cardsEl.innerHTML =
    card('品种数', rows.length) +
    card('全绿品种', rows.length - withMiss) +
    card('有缺口品种', withMiss) +
    card('窗口交易日', g.days.length) +
    card('总覆盖度', (cov * 100).toFixed(2) + '%') +
    card('窗口内无数据品种', absent.length);

  var h = '<h3>市场级缺失日 TOP 15</h3><table><tr><th>日期</th><th>缺失品种数</th></tr>';
  var top = mbd.slice(0, 15);
  for (var j = 0; j < top.length; j++) {
    h += '<tr><td>' + g.days[top[j].i] + '</td><td>' + top[j].n + '</td></tr>';
  }
  if (mbd.length === 0) h += '<tr><td colspan="2">无缺失日 🎉</td></tr>';
  mbdEl.innerHTML = h + '</table>';

  if (absent.length) {
    var names = '';
    for (var k = 0; k < absent.length; k++) {
      var ar = absent[k];
      names += (k ? '　' : '') + ar.s + (ar.n ? ' ' + ar.n : '');
    }
    absentEl.innerHTML = '<details><summary>窗口内无数据的品种（' + absent.length + ' 只）</summary><div class="absent">' + names + '</div></details>';
  } else {
    absentEl.innerHTML = '';
  }
}

function card(k, v) {
  return '<div class="card"><div class="v">' + v + '</div><div class="k">' + k + '</div></div>';
}

function renderWarnings() {
  var all = (R.w || []).concat(grid().warn || []);
  warnEl.innerHTML = all.map(function (w) { return '<div class="warn">⚠ ' + w + '</div>'; }).join('');
}

function renderDetail(ri) {
  var g = grid(), rows = viewRows();
  if (ri < 0 || ri >= rows.length) {
    detailEl.innerHTML = '<div class="empty">点击左侧行查看缺失区段明细</div>';
    return;
  }
  var r = rows[ri];
  var pct = r.e > 0 ? r.p / r.e * 100 : 0;
  var rm = r.m || [];
  var html = '<h3>' + r.s + (r.n ? ' ' + r.n : '') + '</h3>';
  html += '<p>满足度：<b style="color:' + (pct >= 98 ? COLORS.green : pct >= 90 ? COLORS.yellow : COLORS.red) + '">' + pct.toFixed(2) + '%</b>（' + r.p + ' / ' + r.e + ' 个交易日）　数据起点：' + g.days[r.f] + '</p>';
  if (rm.length === 0) {
    html += '<p class="ok">无缺失 ✔</p>';
  } else {
    var missDays = 0;
    for (var i = 0; i < rm.length; i++) missDays += rm[i].l;
    html += '<p class="miss">缺失区段（共 ' + missDays + ' 个交易日）：</p><ul>';
    for (var j = 0; j < rm.length; j++) {
      var s = rm[j];
      html += '<li>' + g.days[s.a] + ' ~ ' + g.days[s.b] + '（' + s.l + ' 个交易日）</li>';
    }
    html += '</ul>';
  }
  var pc = r.pc || [];
  if (g.kind === 'min' && pc.length) {
    html += '<p class="part">部分交易日：</p><ul>';
    for (var k = 0; k < pc.length; k++) {
      html += '<li>' + g.days[pc[k].i] + ' 仅 ' + pc[k].c + ' 分钟</li>';
    }
    html += '</ul>';
  }
  detailEl.innerHTML = html;
}

function refreshAll() {
  zoomInfoEl.textContent = rangeA === null ? '拖拽表头缩放区间，双击表头重置' :
    ('当前区间 ' + grid().days[rangeA] + ' ~ ' + grid().days[rangeB]);
  draw();
}

wrap.addEventListener('scroll', draw);
window.addEventListener('resize', draw);

cv.addEventListener('mousemove', function (e) {
  var p = pos(e);
  var rows = viewRows(), cr = colRange();
  if (p.mx >= geom.labelW && p.my >= geom.headH) {
    var ci = Math.floor((p.mx - geom.labelW) / geom.colW);
    var ri = Math.floor((p.my - geom.headH) / geom.rowH);
    if (ci >= 0 && ci < cr.b - cr.a + 1 && ri >= 0 && ri < rows.length) {
      var r = rows[ri], di = cr.a + ci, g = grid();
      tip.style.display = 'block';
      tip.style.left = (e.clientX + 14) + 'px';
      tip.style.top = (e.clientY + 14) + 'px';
      tip.innerHTML = '<b>' + r.s + '</b> ' + (r.n || '') + '<br>' + g.days[di] + '<br>' + cellStatus(g, r, di);
      return;
    }
  }
  tip.style.display = 'none';
});

cv.addEventListener('mouseleave', function () { tip.style.display = 'none'; });

cv.addEventListener('click', function (e) {
  var p = pos(e);
  if (p.mx < geom.labelW && p.my >= geom.headH) {
    var ri = Math.floor((p.my - geom.headH) / geom.rowH);
    var rows = viewRows();
    if (ri >= 0 && ri < rows.length) {
      selected = ri;
      renderDetail(ri);
      draw();
    }
  }
});

cv.addEventListener('mousedown', function (e) {
  var p = pos(e);
  if (p.my < geom.headH && p.mx >= geom.labelW) {
    var col = Math.floor((p.mx - geom.labelW) / geom.colW);
    drag = { start: col, cur: col };
    draw();
    e.preventDefault();
  }
});

window.addEventListener('mousemove', function (e) {
  if (!drag) return;
  var p = pos(e);
  drag.cur = Math.floor((p.mx - geom.labelW) / geom.colW);
  draw();
});

window.addEventListener('mouseup', function () {
  if (!drag) return;
  var c0 = Math.min(drag.start, drag.cur);
  var c1 = Math.max(drag.start, drag.cur);
  if (c1 - c0 >= 2) {
    var cr = colRange();
    rangeA = cr.a + c0;
    rangeB = cr.a + c1;
  }
  drag = null;
  refreshAll();
});

cv.addEventListener('dblclick', function () { rangeA = null; rangeB = null; refreshAll(); });

document.getElementById('zoomBtn').addEventListener('click', function () {
  rangeA = null; rangeB = null; refreshAll();
});

onlyMissEl.addEventListener('change', function () {
  onlyMiss = onlyMissEl.checked; selected = -1; refreshAll();
});

searchEl.addEventListener('input', function () {
  search = searchEl.value; selected = -1; refreshAll();
});

var tabs = document.querySelectorAll('.tab');
for (var t = 0; t < tabs.length; t++) {
  tabs[t].addEventListener('click', function () {
    kind = this.getAttribute('data-kind');
    for (var i = 0; i < tabs.length; i++) tabs[i].classList.toggle('on', tabs[i] === this);
    selected = -1; rangeA = null; rangeB = null;
    renderWarnings(); renderSummary(); renderDetail(-1); refreshAll();
  });
}

metaEl.textContent = '生成时间：' + R.t + '　·　日线窗口 ' + R.daily.days[0] + ' ~ ' + R.daily.days[R.daily.days.length - 1] +
  '　·　分时窗口 ' + R.min.days[0] + ' ~ ' + R.min.days[R.min.days.length - 1];
renderWarnings();
renderSummary();
refreshAll();

// 支持 #s=<px> / ?s=<px> 直接定位纵向滚动位置（例如 report.html?s=5000）
var hs = location.hash.match(/^#s=(\d+)/) || location.search.match(/[?&]s=(\d+)/);
if (hs) wrap.scrollTop = parseInt(hs[1], 10);
</script>
</body>
</html>
`
