(function() {
  'use strict';

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
    fetch('/api/runs?id=' + runId)
      .then(r => r.json())
      .then(data => {
        document.getElementById('modal-log').textContent = data.log || '(no log)';
        document.getElementById('modal').classList.remove('hidden');
      });
  }

  // SSE
  const evtSource = new EventSource('/api/events');
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
    fetch('/api/grid').then(r => r.json()).then(cells => {
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

  // Summary
  function loadSummary() {
    fetch('/api/summary').then(r => r.json()).then(data => {
      const el = document.getElementById('summary');
      let html = '<h2>Best Flags</h2><table><tr><th>Flags</th><th>Avg Winning</th><th>Win Rate</th><th>Count</th></tr>';
      (data.flags || []).forEach(f => {
        const cls = parseFloat(f.avg_winning) >= 0 ? 'positive' : 'negative';
        html += '<tr><td style="text-align:left">' + (f.flags || '(baseline)') + '</td>' +
          '<td class="' + cls + '">' + f.avg_winning + '</td>' +
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
    fetch('/api/flagsets').then(r => r.json()).then(sets => {
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
    fetch('/api/flagsets', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({name, flag, value})
    }).then(() => loadFlags());
  };

  window.deleteFlag = function(id) {
    fetch('/api/flagsets?id=' + id, {method: 'DELETE'}).then(() => loadFlags());
  };

  window.regenerate = function() {
    fetch('/api/regenerate', {method: 'POST'})
      .then(r => r.json())
      .then(data => {
        alert('Created ' + data.created + ' pending runs');
        loadTab('status');
      });
  };

  // Status
  function updateStatus(state) {
    const el = document.getElementById('status');
    if (!el.classList.contains('active')) return;
    let html = '<h2>Running</h2><table><tr><th>ID</th><th>Symbol</th><th>Date</th><th>Flags</th></tr>';
    (state.active || []).forEach(r => {
      html += '<tr><td>' + r.id + '</td><td class="symbol">' + r.symbol + '</td>' +
        '<td>' + r.date + '</td><td style="text-align:left">' + r.flags + '</td></tr>';
    });
    html += '</table>';

    html += '<h2 style="margin-top:16px">Recent</h2><table><tr><th>ID</th><th>Symbol</th><th>Date</th><th>Flags</th><th>Status</th><th>Winning</th></tr>';
    (state.recent || []).forEach(r => {
      const cls = r.status === 'done' ? (parseFloat(r.winning) >= 0 ? 'positive' : 'negative') : 'pending';
      html += '<tr class="clickable" onclick="showRunLog(' + r.id + ')">' +
        '<td>' + r.id + '</td><td class="symbol">' + r.symbol + '</td>' +
        '<td>' + r.date + '</td><td style="text-align:left">' + r.flags + '</td>' +
        '<td class="' + r.status + '">' + r.status + '</td>' +
        '<td class="' + cls + '">' + (r.winning || '-') + '</td></tr>';
    });
    html += '</table>';
    el.innerHTML = html;
  }

  function loadStatus() {
    fetch('/api/state').then(r => r.json()).then(data => {
      updateCounts(data.counts);
      updateStatus(data);
    });
  }

  window.showRunLog = function(id) { showLog(id); };

  // Initial load — restore tab from hash or default to summary
  var initTab = location.hash.replace('#', '') || 'summary';
  switchTab(initTab);
})();
