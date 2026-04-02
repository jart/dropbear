(function() {
  'use strict';

  var BASE = window.BASE || '';

  // Date range filter with localStorage persistence
  var dateFrom = document.getElementById('date-from');
  var dateTo = document.getElementById('date-to');
  var savedFrom = localStorage.getItem('varulab-date-from');
  var savedTo = localStorage.getItem('varulab-date-to');

  var symbolButtons = document.getElementById('symbol-buttons');
  var selectedSymbol = localStorage.getItem('varulab-symbol') || '';

  fetch(BASE + '/api/dates').then(r => r.json()).then(data => {
    if (data.min) dateFrom.min = data.min;
    if (data.max) dateTo.max = data.max;
    dateFrom.value = savedFrom || data.min || '';
    dateTo.value = savedTo || data.max || '';
    // populate symbol buttons
    symbolButtons.innerHTML = '';
    var allBtn = document.createElement('button');
    allBtn.className = 'sym-btn' + (selectedSymbol === '' ? ' active' : '');
    allBtn.textContent = 'All';
    allBtn.addEventListener('click', function() { selectSymbol(''); });
    symbolButtons.appendChild(allBtn);
    (data.symbols || []).forEach(s => {
      var btn = document.createElement('button');
      btn.className = 'sym-btn' + (s === selectedSymbol ? ' active' : '');
      btn.textContent = s;
      btn.addEventListener('click', function() { selectSymbol(s); });
      symbolButtons.appendChild(btn);
    });
    // now that filters are populated, load the active tab
    var initTab = location.hash.replace('#', '') || 'summary';
    switchTab(initTab);
  });

  function selectSymbol(s) {
    selectedSymbol = s;
    localStorage.setItem('varulab-symbol', s);
    symbolButtons.querySelectorAll('.sym-btn').forEach(b => {
      b.classList.toggle('active', (s === '' && b.textContent === 'All') || b.textContent === s);
    });
    var activeTab = location.hash.replace('#', '') || 'summary';
    loadTab(activeTab);
  }

  function onFilterChange() {
    localStorage.setItem('varulab-date-from', dateFrom.value);
    localStorage.setItem('varulab-date-to', dateTo.value);
    var activeTab = location.hash.replace('#', '') || 'summary';
    loadTab(activeTab);
  }
  dateFrom.addEventListener('change', onFilterChange);
  dateTo.addEventListener('change', onFilterChange);

  function filterParams() {
    var p = '';
    if (dateFrom.value) p += '&from=' + dateFrom.value;
    if (dateTo.value) p += '&to=' + dateTo.value;
    if (selectedSymbol) p += '&symbol=' + selectedSymbol;
    return p;
  }

  // Tab switching with hash persistence
  function switchTab(tab) {
    document.querySelectorAll('.tab').forEach(b => b.classList.remove('active'));
    document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
    var btn = document.querySelector('.tab[data-tab="' + tab + '"]');
    if (btn) btn.classList.add('active');
    var panel = document.getElementById(tab);
    if (panel) panel.classList.add('active');
    location.hash = tab;
    loadTab(tab);
  }
  document.querySelectorAll('.tab').forEach(btn => {
    btn.addEventListener('click', () => switchTab(btn.dataset.tab));
  });

  // Modal
  var modal = document.getElementById('modal');
  document.querySelector('.modal-close').addEventListener('click', () => {
    modal.classList.add('hidden');
  });
  modal.addEventListener('click', (e) => {
    if (e.target === modal) modal.classList.add('hidden');
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') modal.classList.add('hidden');
  });

  function showLog(runId) {
    fetch(BASE + '/api/runs?id=' + runId)
      .then(r => r.json())
      .then(data => {
        document.getElementById('modal-log').textContent = data.log || '(no log)';
        document.getElementById('modal').classList.remove('hidden');
      });
  }

  // SSE
  const evtSource = new EventSource(BASE + '/api/events');
  evtSource.onmessage = function(e) {
    const data = JSON.parse(e.data);
    if (data.type === 'state') {
      updateCounts(data.counts);
      updateStatus(data);
    }
  };

  function updateCounts(c) {
    if (!c) return;
    document.getElementById('counts').textContent =
      'Running: ' + c.running + ' | Pending: ' + c.pending +
      ' | Done: ' + c.done + ' | Failed: ' + c.failed;
  }

  // Tab loaders
  function loadTab(tab) {
    switch (tab) {
      case 'grid': loadGrid(); break;
      case 'summary': loadSummary(); break;
      case 'status': loadStatus(); break;
    }
  }

  // Grid
  function loadGrid() {
    fetch(BASE + '/api/grid?' + filterParams()).then(r => r.json()).then(cells => {
      const el = document.getElementById('grid');
      if (!cells || cells.length === 0) {
        el.innerHTML = '<p>No completed runs yet.</p>';
        return;
      }
      // collect symbols and dates
      const symbols = new Set();
      const dates = new Set();
      const map = {};
      cells.forEach(c => {
        symbols.add(c.symbol);
        dates.add(c.date);
        map[c.symbol + '|' + c.date] = c;
      });
      const sortedSymbols = Array.from(symbols).sort();
      const sortedDates = Array.from(dates).sort().reverse();

      let html = '<table><tr><th></th>';
      sortedDates.forEach(d => { html += '<th>' + d.slice(5) + '</th>'; });
      html += '</tr>';
      sortedSymbols.forEach(sym => {
        html += '<tr><td class="symbol">' + sym + '</td>';
        sortedDates.forEach(d => {
          const c = map[sym + '|' + d];
          if (!c) {
            html += '<td class="pending">-</td>';
          } else {
            const v = parseFloat(c.winning);
            const cls = cellClass(v);
            html += '<td class="clickable ' + cls + '" data-run="' + c.run_id + '">' +
              c.winning + '</td>';
          }
        });
        html += '</tr>';
      });
      html += '</table>';
      el.innerHTML = html;

      el.querySelectorAll('.clickable').forEach(td => {
        td.addEventListener('click', () => showLog(td.dataset.run));
      });
    });
  }

  function cellClass(v) {
    if (v > 5000) return 'cell-green-3';
    if (v > 1000) return 'cell-green-2';
    if (v > 0) return 'cell-green-1';
    if (v > -1000) return 'cell-red-1';
    if (v > -5000) return 'cell-red-2';
    return 'cell-red-3';
  }

  // Spark histogram: sorted values rendered as vertical bars
  function sparkHist(values) {
    if (!values || values.length === 0) return '';
    var w = 200, h = 40;
    var sorted = values.slice().sort(function(a, b) { return a - b; });
    var maxAbs = Math.max(Math.abs(sorted[0]), Math.abs(sorted[sorted.length - 1]), 1);
    var mid = h / 2;
    var barW = Math.max(1, Math.floor(w / sorted.length));
    var bars = '';
    for (var i = 0; i < sorted.length; i++) {
      var v = sorted[i];
      var barH = Math.abs(v) / maxAbs * mid;
      var x = i * barW;
      var color = v >= 0 ? '#98c379' : '#e06c75';
      if (v >= 0) {
        bars += '<rect x="' + x + '" y="' + (mid - barH) + '" width="' + barW + '" height="' + barH + '" fill="' + color + '"/>';
      } else {
        bars += '<rect x="' + x + '" y="' + mid + '" width="' + barW + '" height="' + barH + '" fill="' + color + '"/>';
      }
    }
    return '<svg width="' + (sorted.length * barW) + '" height="' + h + '" style="vertical-align:middle">' +
      '<line x1="0" y1="' + mid + '" x2="' + (sorted.length * barW) + '" y2="' + mid + '" stroke="#333" stroke-width="1"/>' +
      bars + '</svg>';
  }

  // Summary
  function loadSummary() {
    fetch(BASE + '/api/summary?' + filterParams()).then(r => r.json()).then(data => {
      const el = document.getElementById('summary');
      let html = '<h2>Best Flags</h2><table><tr><th>Flags</th><th>Distribution</th><th>Avg</th><th>Fees</th><th>Median</th><th>Best</th><th>Worst</th><th>StdDev</th><th>Sharpe</th><th>Win Rate</th><th>Count</th></tr>';
      (data.flags || []).forEach(f => {
        const cls = parseFloat(f.avg_winning) >= 0 ? 'positive' : 'negative';
        const bcls = parseFloat(f.best_win) >= 0 ? 'positive' : 'negative';
        const wcls = parseFloat(f.worst_loss) >= 0 ? 'positive' : 'negative';
        const mcls = parseFloat(f.median) >= 0 ? 'positive' : 'negative';
        html += '<tr><td style="text-align:left">' + (f.flags || '(baseline)') + '</td>' +
          '<td style="padding:2px">' + sparkHist(f.histogram || []) + '</td>' +
          '<td class="' + cls + '">' + f.avg_winning + '</td>' +
          '<td class="negative">' + f.avg_fees + '</td>' +
          '<td class="' + mcls + '">' + f.median + '</td>' +
          '<td class="' + bcls + '">' + f.best_win + '</td>' +
          '<td class="' + wcls + '">' + f.worst_loss + '</td>' +
          '<td>' + f.std_dev + '</td>' +
          '<td>' + f.sharpe + '</td>' +
          '<td>' + f.win_rate + '</td><td>' + f.count + '</td></tr>';
      });
      html += '</table>';

      html += '<h2 style="margin-top:16px">Best Per Symbol</h2><table><tr><th>Symbol</th><th>Best Flags</th><th>Avg Winning</th><th>Count</th></tr>';
      (data.symbols || []).forEach(s => {
        const cls = parseFloat(s.avg_winning) >= 0 ? 'positive' : 'negative';
        html += '<tr><td class="symbol">' + s.symbol + '</td>' +
          '<td style="text-align:left">' + (s.best_flags || '(baseline)') + '</td>' +
          '<td class="' + cls + '">' + s.avg_winning + '</td>' +
          '<td>' + s.count + '</td></tr>';
      });
      html += '</table>';
      el.innerHTML = html;
    });
  }

  // Flags
  function loadFlags() {
    fetch(BASE + '/api/flagsets').then(r => r.json()).then(sets => {
      const el = document.getElementById('flags');
      let html = '<div class="flag-form">' +
        '<input id="fs-name" placeholder="name (e.g. spread)">' +
        '<input id="fs-flag" placeholder="flag (e.g. -spread)">' +
        '<input id="fs-value" placeholder="value (e.g. 2)">' +
        '<button onclick="addFlag()">Add</button>' +
        '</div>';
      html += '<table><tr><th>Name</th><th>Flag</th><th>Value</th><th></th></tr>';
      (sets || []).forEach(fs => {
        html += '<tr><td style="text-align:left">' + fs.name + '</td>' +
          '<td style="text-align:left">' + fs.flag + '</td>' +
          '<td style="text-align:left">' + (fs.value || '(bool)') + '</td>' +
          '<td><button class="delete-btn" onclick="deleteFlag(' + fs.id + ')">x</button></td></tr>';
      });
      html += '</table>';
      html += '<button class="regen-btn" onclick="regenerate()">Regenerate Runs</button>';
      el.innerHTML = html;
    });
  }

  window.addFlag = function() {
    const name = document.getElementById('fs-name').value;
    const flag = document.getElementById('fs-flag').value;
    const value = document.getElementById('fs-value').value;
    if (!name || !flag) return;
    fetch(BASE + '/api/flagsets', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({name, flag, value})
    }).then(() => loadFlags());
  };

  window.deleteFlag = function(id) {
    fetch(BASE + '/api/flagsets?id=' + id, {method: 'DELETE'}).then(() => loadFlags());
  };

  window.regenerate = function() {
    fetch(BASE + '/api/regenerate', {method: 'POST'})
      .then(r => r.json())
      .then(data => {
        alert('Created ' + data.created + ' pending runs');
        loadTab('status');
      });
  };

  // Status
  function ago(nanos) {
    if (!nanos) return '';
    var ms = (Date.now() * 1e6 - nanos) / 1e6;
    var s = Math.floor(ms / 1000);
    if (s < 60) return s + 's ago';
    var m = Math.floor(s / 60);
    s = s % 60;
    if (m < 60) return m + 'm' + s + 's ago';
    var h = Math.floor(m / 60);
    m = m % 60;
    return h + 'h' + m + 'm ago';
  }

  function updateStatus(state) {
    const el = document.getElementById('status');
    if (!el.classList.contains('active')) return;
    let html = '<h2>Running (' + (state.active || []).length + ')</h2><table><tr><th>ID</th><th>Symbol</th><th>Date</th><th>Flags</th><th>Started</th></tr>';
    (state.active || []).forEach(r => {
      html += '<tr><td>' + r.id + '</td><td class="symbol">' + r.symbol + '</td>' +
        '<td>' + r.date + '</td><td style="text-align:left">' + r.flags + '</td>' +
        '<td>' + ago(r.started_at) + '</td></tr>';
    });
    html += '</table>';

    html += '<h2 style="margin-top:16px">Pending (' + (state.counts ? state.counts.pending : '?') + ')</h2><table><tr><th>ID</th><th>Symbol</th><th>Date</th><th>Flags</th></tr>';
    (state.pending || []).forEach(r => {
      html += '<tr><td>' + r.id + '</td><td class="symbol">' + r.symbol + '</td>' +
        '<td>' + r.date + '</td><td style="text-align:left">' + r.flags + '</td></tr>';
    });
    html += '</table>';

    html += '<h2 style="margin-top:16px">Recent Completed</h2><table><tr><th>ID</th><th>Symbol</th><th>Date</th><th>Flags</th><th>Status</th><th>Winning</th><th>Finished</th></tr>';
    (state.recent || []).forEach(r => {
      const cls = r.status === 'done' ? (parseFloat(r.winning) >= 0 ? 'positive' : 'negative') : 'pending';
      html += '<tr class="clickable" onclick="showRunLog(' + r.id + ')">' +
        '<td>' + r.id + '</td><td class="symbol">' + r.symbol + '</td>' +
        '<td>' + r.date + '</td><td style="text-align:left">' + r.flags + '</td>' +
        '<td class="' + r.status + '">' + r.status + '</td>' +
        '<td class="' + cls + '">' + (r.winning || '-') + '</td>' +
        '<td>' + ago(r.finished_at) + '</td></tr>';
    });
    html += '</table>';

    if ((state.failed || []).length > 0) {
      html += '<h2 style="margin-top:16px">Failed (' + (state.counts ? state.counts.failed : '?') + ')</h2><table><tr><th>ID</th><th>Symbol</th><th>Date</th><th>Flags</th><th>Failed</th></tr>';
      (state.failed || []).forEach(r => {
        html += '<tr class="clickable" onclick="showRunLog(' + r.id + ')">' +
          '<td>' + r.id + '</td><td class="symbol">' + r.symbol + '</td>' +
          '<td>' + r.date + '</td><td style="text-align:left">' + r.flags + '</td>' +
          '<td>' + ago(r.finished_at) + '</td></tr>';
      });
      html += '</table>';
    }

    el.innerHTML = html;
  }

  function loadStatus() {
    fetch(BASE + '/api/state').then(r => r.json()).then(data => {
      updateCounts(data.counts);
      updateStatus(data);
    });
  }

  window.showRunLog = function(id) { showLog(id); };

  // Initial load happens inside the /api/dates callback above
})();
