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
  document.getElementById('price').textContent = d.price;
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
  colorize('statRealized', d.stats.realized);
  setText('statError', d.stats.error);
  var errEl = document.getElementById('statError');
  errEl.className = d.stats.error == 0 ? 'zero' : 'neg';
  setText('statVolume', d.stats.volume);
}

function setText(id, val) {
  document.getElementById(id).textContent = val;
}

function colorize(id, val) {
  var el = document.getElementById(id);
  el.className = val > 0 ? 'pos' : val < 0 ? 'neg' : 'zero';
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
  var html = '';
  var empty5 = '<td></td><td></td><td></td><td></td><td></td>';
  for (var i = 0; i < strikeList.length; i++) {
    var k = strikeList[i];
    var c = strikes[k]['C'];
    var p = strikes[k]['P'];
    html += '<tr>';
    if (c) {
      var cq = c.qty;
      var ci = c.itm ? ' itm' : '';
      html += '<td class="' + (cq > 0 ? 'pos' : cq < 0 ? 'neg' : 'zero') + ci + '">' + c.qty + '</td>';
      html += '<td class="' + ci + '">' + c.bid + '</td>';
      html += '<td class="' + ci + '">' + c.ask + '</td>';
      html += '<td class="' + ci + '">' + c.mid + '</td>';
      html += '<td class="' + ci + '">' + c.delta + '</td>';
    } else {
      html += empty5;
    }
    html += '<td class="strike-col">' + k + '</td>';
    if (p) {
      var pq = p.qty;
      var pi = p.itm ? ' itm' : '';
      html += '<td class="' + (pq > 0 ? 'pos' : pq < 0 ? 'neg' : 'zero') + pi + '">' + p.qty + '</td>';
      html += '<td class="' + pi + '">' + p.bid + '</td>';
      html += '<td class="' + pi + '">' + p.ask + '</td>';
      html += '<td class="' + pi + '">' + p.mid + '</td>';
      html += '<td class="' + pi + '">' + p.delta + '</td>';
    } else {
      html += empty5;
    }
    html += '</tr>';
  }
  body.innerHTML = html;
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
  var html = '';
  for (var i = 0; i < orders.length; i++) {
    var o = orders[i];
    var qc = o.qty > 0 ? 'pos' : 'neg';
    html += '<tr>';
    html += '<td>' + o.id + '</td>';
    html += '<td>' + o.security + '</td>';
    html += '<td class="' + qc + '">' + o.qty + '</td>';
    html += '<td>' + o.price + '</td>';
    html += '<td>' + o.mid + '</td>';
    html += '<td>' + o.status + '</td>';
    html += '<td>' + o.age + '</td>';
    html += '</tr>';
  }
  if (!orders.length) html = '<tr><td colspan="7" style="text-align:center;color:#666">none</td></tr>';
  body.innerHTML = html;
}

function renderTransactions(txs) {
  var body = document.getElementById('txBody');
  var html = '';
  for (var i = 0; i < txs.length; i++) {
    var tx = txs[i];
    var qc = tx.qty > 0 ? 'pos' : 'neg';
    var time = tx.time.length > 19 ? tx.time.substring(11, 19) : tx.time;
    html += '<tr>';
    html += '<td>' + time + '</td>';
    html += '<td>' + tx.security + '</td>';
    html += '<td class="' + qc + '">' + tx.qty + '</td>';
    html += '<td>' + tx.price + '</td>';
    html += '<td>' + tx.id + '</td>';
    html += '</tr>';
  }
  if (!txs.length) html = '<tr><td colspan="5" style="text-align:center;color:#666">none</td></tr>';
  body.innerHTML = html;
}

function renderFlags(flags) {
  if (!flags) return;
  var form = document.getElementById('flagsForm');
  setIfEmpty(form.straddles, flags.straddles);
  setIfEmpty(form.tolerance, flags.tolerance);
  setIfEmpty(form.patience, flags.patience);
  setIfEmpty(form.quantum, flags.quantum);
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
    straddles: form.straddles.value,
    tolerance: form.tolerance.value,
    patience: form.patience.value,
    quantum: form.quantum.value,
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
