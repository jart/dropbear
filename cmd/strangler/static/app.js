'use strict';

function connect() {
  var status = document.getElementById('status');
  var es = new EventSource('/api/events');
  es.onmessage = function(e) {
    status.textContent = 'live';
    status.className = 'status-connected';
    var data = JSON.parse(e.data);
    if (data && data.symbol)
      render(data);
  };
  es.onerror = function() {
    status.textContent = 'disconnected';
    status.className = 'status-disconnected';
  };
}

function render(d) {
  document.title = d.symbol + ' strangler';
  document.getElementById('symbol').textContent = d.symbol;
  document.getElementById('price').textContent = fmtNum(d.price);
  document.getElementById('time').textContent = d.time;
  document.getElementById('pauseBtn').disabled = d.paused;
  document.getElementById('resumeBtn').disabled = !d.paused;
  renderStats(d);
  renderPositions(d.positions || []);
  renderRiskChart(d);
  renderPendingOrders(d.pendingOrders || []);
  renderTransactions(d.transactions || []);
  renderFlags(d.flags);
}

function renderStats(d) {
  setText('statLiq', d.stats.liquidation);
  setText('statWorst', d.stats.worst);
  setText('statRealized', d.stats.realized);
  setText('statCash', d.cash);
  setText('statFees', d.stats.fees);
  setText('statShares', d.stats.shares);
  setText('statShareCost', d.stats.shareCost);
  setText('statOrders', d.stats.orders);
  setText('statDelta', d.greeks.delta);
  setText('statGamma', d.greeks.gamma);
  setText('statTheta', d.greeks.theta);
  setText('statVega', d.greeks.vega);
  colorize('statLiq', d.stats.liquidation);
  colorize('statWorst', d.stats.worst);
  setText('statEOD', d.stats.eod);
  colorize('statEOD', d.stats.eod);
  colorize('statRealized', d.stats.realized);
  setText('statError', d.stats.error);
  var errEl = document.getElementById('statError');
  errEl.className = d.stats.error == 0 ? 'zero' : 'neg';
  setText('statVolume', d.stats.volume);
}

function fmtNum(val) {
  if (typeof val !== 'number') return val;
  var s = val.toFixed(6);
  var dot = s.indexOf('.');
  var minLen = dot + 3;
  var i = s.length;
  while (i > minLen && s[i - 1] === '0') i--;
  s = s.substring(0, i);
  var parts = s.split('.');
  parts[0] = parts[0].replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  return parts.join('.');
}

function setText(id, val) {
  document.getElementById(id).textContent = fmtNum(val);
}

function colorize(id, val) {
  var el = document.getElementById(id);
  el.className = val > 0 ? 'pos' : val < 0 ? 'neg' : 'zero';
}

function td(text, cls) {
  var el = document.createElement('td');
  if (cls) el.className = cls;
  el.textContent = text;
  return el;
}

function qtyClass(qty) {
  return qty > 0 ? 'pos' : qty < 0 ? 'neg' : 'zero';
}

function optionCells(tr, opt) {
  if (opt) {
    var cls = opt.itm ? ' itm' : '';
    tr.appendChild(td(opt.qty, qtyClass(opt.qty) + cls));
    tr.appendChild(td(fmtNum(opt.bid), cls));
    tr.appendChild(td(fmtNum(opt.ask), cls));
    tr.appendChild(td(fmtNum(opt.mid), cls));
    tr.appendChild(td(fmtNum(opt.delta), cls));
    tr.appendChild(td(fmtNum(opt.iv), cls));
  } else {
    for (var j = 0; j < 6; j++) tr.appendChild(td(''));
  }
}

function renderPositions(positions) {
  var body = document.getElementById('positionsBody');
  var strikes = {};
  var strikeList = [];
  for (var i = 0; i < positions.length; i++) {
    var p = positions[i];
    var k = p.strike;
    if (!strikes[k]) {
      strikes[k] = {};
      strikeList.push(k);
    }
    strikes[k][p.class] = p;
  }
  strikeList.sort(function(a, b) { return a - b; });
  while (body.firstChild) body.removeChild(body.firstChild);
  for (var i = 0; i < strikeList.length; i++) {
    var k = strikeList[i];
    var tr = document.createElement('tr');
    optionCells(tr, strikes[k]['C']);
    tr.appendChild(td(fmtNum(k), 'strike-col'));
    optionCells(tr, strikes[k]['P']);
    body.appendChild(tr);
  }
}

function renderRiskChart(d) {
  var risk = d.risk || [];
  var canvas = document.getElementById('riskChart');
  var ctx = canvas.getContext('2d');
  var w = canvas.width;
  var h = canvas.height;
  ctx.clearRect(0, 0, w, h);
  if (risk.length < 2) return;

  var price = d.price;
  var sigma = d.sigma;
  var strikes = [];
  var settlements = [];
  for (var i = 0; i < risk.length; i++) {
    strikes.push(risk[i].strike);
    settlements.push(risk[i].settlement);
  }

  var xMin = strikes[0], xMax = strikes[strikes.length - 1];
  var yMin = Math.min.apply(null, settlements);
  var yMax = Math.max.apply(null, settlements);
  var yPad = (yMax - yMin) * 0.1 || 100;
  yMin -= yPad;
  yMax += yPad;

  function tx(x) { return (x - xMin) / (xMax - xMin) * w; }
  function ty(y) { return h - (y - yMin) / (yMax - yMin) * h; }

  // zero line
  if (yMin < 0 && yMax > 0) {
    ctx.strokeStyle = '#333';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(0, ty(0));
    ctx.lineTo(w, ty(0));
    ctx.stroke();
  }

  // sigma lines
  if (sigma > 0) {
    ctx.lineWidth = 1;
    for (var n = 1; n <= 3; n++) {
      var alpha = 0.6 / n;
      ctx.strokeStyle = 'rgba(255,235,59,' + alpha + ')';
      var lo = price - sigma * n;
      var hi = price + sigma * n;
      if (lo >= xMin) {
        ctx.beginPath();
        ctx.moveTo(tx(lo), 0);
        ctx.lineTo(tx(lo), h);
        ctx.stroke();
      }
      if (hi <= xMax) {
        ctx.beginPath();
        ctx.moveTo(tx(hi), 0);
        ctx.lineTo(tx(hi), h);
        ctx.stroke();
      }
    }
  }

  // current price line
  if (price >= xMin && price <= xMax) {
    ctx.strokeStyle = '#ef5350';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(tx(price), 0);
    ctx.lineTo(tx(price), h);
    ctx.stroke();
  }

  // P&L line
  ctx.strokeStyle = '#4fc3f7';
  ctx.lineWidth = 2;
  ctx.beginPath();
  for (var i = 0; i < strikes.length; i++) {
    var x = tx(strikes[i]);
    var y = ty(settlements[i]);
    if (i === 0) ctx.moveTo(x, y);
    else ctx.lineTo(x, y);
  }
  ctx.stroke();

  // fill positive green, negative red
  for (var i = 0; i < strikes.length - 1; i++) {
    var x1 = tx(strikes[i]), x2 = tx(strikes[i + 1]);
    var y1 = settlements[i], y2 = settlements[i + 1];
    var avg = (y1 + y2) / 2;
    ctx.fillStyle = avg >= 0 ? 'rgba(102,187,106,0.15)' : 'rgba(239,83,80,0.15)';
    ctx.beginPath();
    ctx.moveTo(x1, ty(y1));
    ctx.lineTo(x2, ty(y2));
    ctx.lineTo(x2, ty(0));
    ctx.lineTo(x1, ty(0));
    ctx.closePath();
    ctx.fill();
  }

  // y-axis labels
  ctx.fillStyle = '#888';
  ctx.font = '11px monospace';
  ctx.textAlign = 'left';
  var ySteps = 5;
  for (var i = 0; i <= ySteps; i++) {
    var val = yMin + (yMax - yMin) * i / ySteps;
    ctx.fillText(Math.round(val).toString(), 4, ty(val) - 2);
  }

  // x-axis labels
  ctx.textAlign = 'center';
  var xSteps = Math.min(10, strikes.length);
  var step = Math.floor(strikes.length / xSteps);
  for (var i = 0; i < strikes.length; i += step) {
    ctx.fillText(strikes[i].toString(), tx(strikes[i]), h - 4);
  }
}

function renderPendingOrders(orders) {
  var body = document.getElementById('ordersBody');
  while (body.firstChild) body.removeChild(body.firstChild);
  if (!orders.length) {
    var tr = document.createElement('tr');
    var cell = td('none');
    cell.colSpan = 7;
    cell.style.textAlign = 'center';
    cell.style.color = '#666';
    tr.appendChild(cell);
    body.appendChild(tr);
    return;
  }
  for (var i = 0; i < orders.length; i++) {
    var o = orders[i];
    var tr = document.createElement('tr');
    tr.appendChild(td(o.id));
    tr.appendChild(td(o.security, 'left'));
    tr.appendChild(td(fmtNum(o.qty), qtyClass(o.qty)));
    tr.appendChild(td(fmtNum(o.price)));
    tr.appendChild(td(fmtNum(o.mid)));
    tr.appendChild(td(o.status));
    tr.appendChild(td(o.age));
    body.appendChild(tr);
  }
}

function renderTransactions(txs) {
  var body = document.getElementById('txBody');
  while (body.firstChild) body.removeChild(body.firstChild);
  if (!txs.length) {
    var tr = document.createElement('tr');
    var cell = td('none');
    cell.colSpan = 8;
    cell.style.textAlign = 'center';
    cell.style.color = '#666';
    tr.appendChild(cell);
    body.appendChild(tr);
    return;
  }
  for (var i = 0; i < txs.length; i++) {
    var tx = txs[i];
    var time = tx.time.length > 19 ? tx.time.substring(11, 19) : tx.time;
    var tr = document.createElement('tr');
    tr.appendChild(td(time, 'left'));
    tr.appendChild(td(tx.security, 'left'));
    tr.appendChild(td(fmtNum(tx.qty)));
    tr.appendChild(td(fmtNum(tx.limit)));
    tr.appendChild(td(fmtNum(tx.fill)));
    var impCls = tx.improvement > 0 ? 'pos' : tx.improvement < 0 ? 'neg' : '';
    tr.appendChild(td(tx.improvement ? fmtNum(tx.improvement) : '', impCls));
    var pnlCls = tx.pnl > 0 ? 'pos' : tx.pnl < 0 ? 'neg' : '';
    tr.appendChild(td(tx.pnl ? fmtNum(tx.pnl) : '', pnlCls));
    tr.appendChild(td(fmtNum(tx.fee)));
    body.appendChild(tr);
  }
}

function renderFlags(flags) {
  if (!flags) return;
  var form = document.getElementById('flagsForm');
  setIfEmpty(form.contracts, flags.contracts);
  setIfEmpty(form.tolerance, flags.tolerance);
  setIfEmpty(form.patience, flags.patience);
  setIfEmpty(form.spread, flags.spread);
  setIfEmpty(form.risk, flags.risk);
}

function setIfEmpty(input, val) {
  if (document.activeElement !== input && input.value === '') {
    input.value = val;
  }
}

function pause() {
  fetch('/api/pause', { method: 'POST' });
}

function resume() {
  fetch('/api/resume', { method: 'POST' });
}

function submitFlags(e) {
  e.preventDefault();
  var form = document.getElementById('flagsForm');
  var data = {
    contracts: form.contracts.value,
    tolerance: form.tolerance.value,
    patience: form.patience.value,
    spread: form.spread.value,
    risk: form.risk.value
  };
  fetch('/api/flags', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data)
  });
  return false;
}

connect();
