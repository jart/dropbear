'use strict';

var pollTimer = null;

function startPolling() {
  var status = document.getElementById('status');
  function poll() {
    fetch('/api/state')
      .then(function(r) { return r.json(); })
      .then(function(data) {
        status.textContent = 'live';
        status.className = 'status-connected';
        if (data && data.symbol) render(data);
      })
      .catch(function() {
        status.textContent = 'disconnected';
        status.className = 'status-disconnected';
      });
  }
  poll();
  pollTimer = setInterval(poll, 250);
}

function render(d) {
  document.getElementById('symbol').textContent = d.symbol;
  document.getElementById('price').textContent = d.price;
  document.getElementById('time').textContent = d.time;
  document.getElementById('cash').textContent = '$' + d.cash;
  document.getElementById('pauseBtn').disabled = d.paused;
  document.getElementById('resumeBtn').disabled = !d.paused;
  renderStats(d);
  renderPositions(d.positions || []);
  renderRiskChart(d);
  renderStrategies(d.strategies || {});
  renderFlags(d.flags);
}

function renderStats(d) {
  setText('statPayoff', d.stats.payoff);
  setText('statWorst', d.stats.worst);
  setText('statLiq', d.stats.liquidation);
  setText('statEOD', d.stats.eod);
  setText('statSigma', d.stats.sigma);
  setText('statBias', d.stats.bias);
  setText('statDelta', d.greeks.delta);
  setText('statGamma', d.greeks.gamma);
  setText('statTheta', d.greeks.theta);
  setText('statVega', d.greeks.vega);
  colorize('statPayoff', d.stats.payoff);
  colorize('statWorst', d.stats.worst);
  colorize('statEOD', d.stats.eod);
  colorize('statBias', d.stats.bias);
}

function setText(id, val) {
  document.getElementById(id).textContent = val;
}

function colorize(id, val) {
  var el = document.getElementById(id);
  var n = parseFloat(val);
  el.className = n > 0 ? 'pos' : n < 0 ? 'neg' : 'zero';
}

function renderPositions(positions) {
  var body = document.getElementById('positionsBody');
  // group by strike
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
  strikeList.sort(function(a, b) { return parseFloat(a) - parseFloat(b); });
  var html = '';
  var empty6 = '<td></td><td></td><td></td><td></td><td></td><td></td>';
  for (var i = 0; i < strikeList.length; i++) {
    var k = strikeList[i];
    var c = strikes[k]['C'];
    var p = strikes[k]['P'];
    html += '<tr>';
    // calls (left side)
    if (c) {
      var cq = parseFloat(c.qty);
      var ci = c.itm ? ' itm' : '';
      html += '<td class="' + (cq > 0 ? 'pos' : cq < 0 ? 'neg' : 'zero') + ci + '">' + c.qty + '</td>';
      html += '<td class="' + ci + '">' + c.bid + '</td>';
      html += '<td class="' + ci + '">' + c.ask + '</td>';
      html += '<td class="' + ci + '">' + c.mid + '</td>';
      html += '<td class="' + ci + '">' + c.delta + '</td>';
      html += '<td class="' + ci + '">' + c.prob + '%</td>';
    } else {
      html += empty6;
    }
    // strike (center)
    html += '<td class="strike-col">' + k + '</td>';
    // puts (right side)
    if (p) {
      var pq = parseFloat(p.qty);
      var pi = p.itm ? ' itm' : '';
      html += '<td class="' + (pq > 0 ? 'pos' : pq < 0 ? 'neg' : 'zero') + pi + '">' + p.qty + '</td>';
      html += '<td class="' + pi + '">' + p.bid + '</td>';
      html += '<td class="' + pi + '">' + p.ask + '</td>';
      html += '<td class="' + pi + '">' + p.mid + '</td>';
      html += '<td class="' + pi + '">' + p.delta + '</td>';
      html += '<td class="' + pi + '">' + p.prob + '%</td>';
    } else {
      html += empty6;
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

  var price = parseFloat(d.price);
  var sigma = parseFloat(d.sigma);
  var strikes = [];
  var settlements = [];
  for (var i = 0; i < risk.length; i++) {
    strikes.push(parseFloat(risk[i].strike));
    settlements.push(parseFloat(risk[i].settlement));
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

  // sigma lines (yellow, solid, fading with distance)
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

  // current price line (red, thin)
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

function renderStrategies(strategies) {
  var div = document.getElementById('strategiesList');
  var names = Object.keys(strategies).sort();
  // only rebuild if structure changed
  if (div.childElementCount !== names.length) {
    var html = '';
    for (var i = 0; i < names.length; i++) {
      var name = names[i];
      var s = strategies[name];
      html += '<label>' +
        '<input type="checkbox" data-strategy="' + name + '"' +
        (s.enabled ? ' checked' : '') +
        ' onchange="toggleStrategy(this)">' +
        name +
        '<span class="count" id="strat-' + name.replace(/ /g, '-') + '">' + s.count + '</span>' +
        '</label>';
    }
    div.innerHTML = html;
  } else {
    // just update counts and checked state
    for (var i = 0; i < names.length; i++) {
      var name = names[i];
      var s = strategies[name];
      var countEl = document.getElementById('strat-' + name.replace(/ /g, '-'));
      if (countEl) countEl.textContent = s.count;
      var cb = div.querySelector('input[data-strategy="' + name + '"]');
      if (cb) cb.checked = s.enabled;
    }
  }
}

function renderFlags(flags) {
  if (!flags) return;
  var form = document.getElementById('flagsForm');
  setIfEmpty(form.sigmas, flags.sigmas);
  setIfEmpty(form.budget, flags.budget);
  setIfEmpty(form.floor, flags.floor);
  setIfEmpty(form.spread, flags.spread);
  setIfEmpty(form.cooldown, flags.cooldown);
  setIfEmpty(form.patience, flags.patience);
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
    sigmas: form.sigmas.value,
    budget: form.budget.value,
    floor: form.floor.value,
    spread: form.spread.value,
    cooldown: form.cooldown.value,
    patience: form.patience.value
  };
  fetch('/api/flags', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data)
  });
  return false;
}

function toggleStrategy(cb) {
  var data = {};
  data[cb.dataset.strategy] = cb.checked;
  fetch('/api/strategies', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data)
  });
}

startPolling();
