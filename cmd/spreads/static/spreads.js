(function () {
    'use strict';

    var select = document.getElementById('instrument');
    var canvas = document.getElementById('chart');
    var ctx = canvas.getContext('2d');
    var tooltip = document.getElementById('tooltip');
    var statusEl = document.getElementById('status');
    var loadingEl = document.getElementById('loading');

    var PRICE_SCALE = 1e9;
    var BID_COLOR = '#4dabf7';
    var ASK_COLOR = '#ff922b';
    var GRID_COLOR = '#0f3460';
    var TEXT_COLOR = '#888';
    var CROSSHAIR_COLOR = '#444';
    var PAD = { top: 20, right: 80, bottom: 40, left: 20 };

    var data = null;
    var dpr = window.devicePixelRatio || 1;

    // Load instruments
    fetch('/api/instruments')
        .then(function (r) { return r.json(); })
        .then(function (instruments) {
            select.innerHTML = '<option value="">Select an instrument...</option>';
            if (!instruments || instruments.length === 0) {
                select.innerHTML = '<option value="">No instruments found</option>';
                return;
            }
            for (var i = 0; i < instruments.length; i++) {
                var inst = instruments[i];
                var opt = document.createElement('option');
                opt.value = inst.id;
                opt.textContent = inst.symbol + ' ' + inst.class + ' $' + inst.strike +
                    ' exp ' + inst.expiration + ' (' + inst.count + ' ticks)';
                select.appendChild(opt);
            }
        })
        .catch(function (err) {
            select.innerHTML = '<option value="">Error loading instruments</option>';
            console.error(err);
        });

    select.addEventListener('change', function () {
        var id = select.value;
        if (!id) {
            data = null;
            draw();
            return;
        }
        loadingEl.textContent = 'Loading NBBO data...';
        fetch('/api/nbbo?id=' + id)
            .then(function (r) { return r.json(); })
            .then(function (nbbo) {
                loadingEl.textContent = '';
                if (!nbbo || nbbo.length === 0) {
                    statusEl.textContent = 'No data for this instrument';
                    data = null;
                    draw();
                    return;
                }
                data = nbbo;
                statusEl.textContent = nbbo.length + ' ticks';
                resize();
                draw();
            })
            .catch(function (err) {
                loadingEl.textContent = '';
                statusEl.textContent = 'Error loading data';
                console.error(err);
            });
    });

    function resize() {
        var rect = canvas.parentElement.getBoundingClientRect();
        var w = rect.width;
        var h = Math.max(400, Math.min(600, window.innerHeight - 200));
        canvas.style.height = h + 'px';
        canvas.width = w * dpr;
        canvas.height = h * dpr;
    }

    function niceStep(range, targetTicks) {
        var rough = range / targetTicks;
        var mag = Math.pow(10, Math.floor(Math.log10(rough)));
        var norm = rough / mag;
        var step;
        if (norm < 1.5) step = 1;
        else if (norm < 3.5) step = 2;
        else if (norm < 7.5) step = 5;
        else step = 10;
        return step * mag;
    }

    function formatTime(nsTimestamp) {
        var ms = nsTimestamp / 1e6;
        var d = new Date(ms);
        var h = d.getUTCHours();
        var m = d.getUTCMinutes();
        var s = d.getUTCSeconds();
        var frac = d.getUTCMilliseconds();
        return pad2(h) + ':' + pad2(m) + ':' + pad2(s) + '.' + pad3(frac);
    }

    function formatTimeShort(nsTimestamp) {
        var ms = nsTimestamp / 1e6;
        var d = new Date(ms);
        return pad2(d.getUTCHours()) + ':' + pad2(d.getUTCMinutes()) + ':' + pad2(d.getUTCSeconds());
    }

    function pad2(n) { return n < 10 ? '0' + n : '' + n; }
    function pad3(n) { return n < 10 ? '00' + n : n < 100 ? '0' + n : '' + n; }

    function draw() {
        var w = canvas.width;
        var h = canvas.height;
        ctx.clearRect(0, 0, w, h);

        if (!data || data.length === 0) return;

        var pl = PAD.left * dpr;
        var pr = PAD.right * dpr;
        var pt = PAD.top * dpr;
        var pb = PAD.bottom * dpr;
        var cw = w - pl - pr;
        var ch = h - pt - pb;

        // Compute price range from string prices
        var minP = Infinity, maxP = -Infinity;
        var minT = data[0].ts, maxT = data[data.length - 1].ts;

        for (var i = 0; i < data.length; i++) {
            var bp = parseFloat(data[i].bid_px);
            var ap = parseFloat(data[i].ask_px);
            if (bp < minP) minP = bp;
            if (ap < minP) minP = ap;
            if (bp > maxP) maxP = bp;
            if (ap > maxP) maxP = ap;
        }

        // Add padding
        var pRange = maxP - minP;
        if (pRange === 0) pRange = 1;
        minP -= pRange * 0.05;
        maxP += pRange * 0.05;
        pRange = maxP - minP;

        var tRange = maxT - minT;
        if (tRange === 0) tRange = 1;

        function px(ts) { return pl + ((ts - minT) / tRange) * cw; }
        function py(price) { return pt + ch - ((price - minP) / pRange) * ch; }

        // Draw grid
        ctx.strokeStyle = GRID_COLOR;
        ctx.lineWidth = dpr;
        ctx.font = (11 * dpr) + 'px system-ui, sans-serif';
        ctx.fillStyle = TEXT_COLOR;
        ctx.textAlign = 'right';

        // Y axis grid (price)
        var pStep = niceStep(pRange, 8);
        var pStart = Math.ceil(minP / pStep) * pStep;
        for (var p = pStart; p <= maxP; p += pStep) {
            var y = py(p);
            ctx.beginPath();
            ctx.moveTo(pl, y);
            ctx.lineTo(w - pr, y);
            ctx.stroke();
            ctx.fillText(p.toFixed(2), w - 4 * dpr, y + 4 * dpr);
        }

        // X axis grid (time)
        ctx.textAlign = 'center';
        var tStep = niceStep(tRange, 6);
        var tStart = Math.ceil(minT / tStep) * tStep;
        for (var t = tStart; t <= maxT; t += tStep) {
            var x = px(t);
            ctx.beginPath();
            ctx.moveTo(x, pt);
            ctx.lineTo(x, h - pb);
            ctx.stroke();
            ctx.fillText(formatTimeShort(t), x, h - pb + 14 * dpr);
        }

        // Draw bid step chart
        drawStepChart(BID_COLOR, 'bid_px', pl, cw, pt, ch, minT, tRange, minP, pRange);

        // Draw ask step chart
        drawStepChart(ASK_COLOR, 'ask_px', pl, cw, pt, ch, minT, tRange, minP, pRange);
    }

    function drawStepChart(color, field, pl, cw, pt, ch, minT, tRange, minP, pRange) {
        ctx.strokeStyle = color;
        ctx.lineWidth = 1.5 * dpr;
        ctx.beginPath();

        var prevX, prevY;
        for (var i = 0; i < data.length; i++) {
            var x = pl + ((data[i].ts - minT) / tRange) * cw;
            var y = pt + ch - ((parseFloat(data[i][field]) - minP) / pRange) * ch;

            if (i === 0) {
                ctx.moveTo(x, y);
            } else {
                // Step: horizontal then vertical
                ctx.lineTo(x, prevY);
                ctx.lineTo(x, y);
            }
            prevX = x;
            prevY = y;
        }
        ctx.stroke();
    }

    // Tooltip on mousemove
    canvas.addEventListener('mousemove', function (e) {
        if (!data || data.length === 0) {
            tooltip.style.display = 'none';
            return;
        }

        var rect = canvas.getBoundingClientRect();
        var mx = e.clientX - rect.left;
        var my = e.clientY - rect.top;
        var w = rect.width;
        var h = rect.height;

        var pl = PAD.left;
        var pr = PAD.right;
        var pt = PAD.top;
        var pb = PAD.bottom;
        var cw = w - pl - pr;

        if (mx < pl || mx > w - pr) {
            tooltip.style.display = 'none';
            return;
        }

        var minT = data[0].ts;
        var maxT = data[data.length - 1].ts;
        var tRange = maxT - minT;
        if (tRange === 0) tRange = 1;

        var tsTarget = minT + ((mx - pl) / cw) * tRange;

        // Binary search for nearest tick
        var lo = 0, hi = data.length - 1;
        while (lo < hi) {
            var mid = (lo + hi) >> 1;
            if (data[mid].ts < tsTarget) lo = mid + 1;
            else hi = mid;
        }
        // Check neighbors
        if (lo > 0 && Math.abs(data[lo - 1].ts - tsTarget) < Math.abs(data[lo].ts - tsTarget)) {
            lo = lo - 1;
        }

        var d = data[lo];
        var bid = parseFloat(d.bid_px);
        var ask = parseFloat(d.ask_px);
        var spread = ask - bid;

        tooltip.querySelector('.time').textContent = formatTime(d.ts);
        tooltip.querySelector('.bid').textContent = 'Bid: ' + bid.toFixed(4) + ' (' + d.bid_sz + ')';
        tooltip.querySelector('.ask').textContent = 'Ask: ' + ask.toFixed(4) + ' (' + d.ask_sz + ')';
        tooltip.querySelector('.spread').textContent = 'Spread: ' + spread.toFixed(4);

        // Position tooltip
        var tx = e.clientX - rect.left + 16;
        var ty = e.clientY - rect.top - 10;
        if (tx + 180 > w) tx = e.clientX - rect.left - 196;
        tooltip.style.left = tx + 'px';
        tooltip.style.top = ty + 'px';
        tooltip.style.display = 'block';

        // Draw crosshair
        draw();
        var cx = (mx / w) * canvas.width;
        ctx.strokeStyle = CROSSHAIR_COLOR;
        ctx.lineWidth = dpr;
        ctx.setLineDash([4 * dpr, 4 * dpr]);
        ctx.beginPath();
        ctx.moveTo(cx, PAD.top * dpr);
        ctx.lineTo(cx, canvas.height - PAD.bottom * dpr);
        ctx.stroke();
        ctx.setLineDash([]);
    });

    canvas.addEventListener('mouseleave', function () {
        tooltip.style.display = 'none';
        draw();
    });

    window.addEventListener('resize', function () {
        resize();
        draw();
    });

    resize();
})();
