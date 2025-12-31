// Chart data and series stored globally for toggle functionality
var chartData = null;
var strategySeries = null;
var benchmarkSeries = null;
var showingFees = true;

// Initialize the chart when data is loaded
fetch('data.json')
    .then(function(response) {
        return response.json();
    })
    .then(function(data) {
        chartData = data;
        var chartContainer = document.getElementById('chart');

        var chart = LightweightCharts.createChart(chartContainer, {
            layout: {
                background: { type: 'solid', color: '#0a0a0a' },
                textColor: '#666',
            },
            grid: {
                vertLines: { color: '#1a1a1a' },
                horzLines: { color: '#1a1a1a' },
            },
            rightPriceScale: {
                borderColor: '#1a1a1a',
            },
            timeScale: {
                borderColor: '#1a1a1a',
                timeVisible: true,
            },
            crosshair: {
                vertLine: {
                    color: '#444',
                    width: 1,
                    style: 2,
                    labelBackgroundColor: '#222',
                },
                horzLine: {
                    color: '#444',
                    width: 1,
                    style: 2,
                    labelBackgroundColor: '#222',
                },
            },
            handleScale: {
                axisPressedMouseMove: {
                    time: true,
                    price: true,
                },
            },
        });

        // Strategy line (cyan)
        strategySeries = chart.addSeries(LightweightCharts.LineSeries, {
            color: '#00d4ff',
            lineWidth: 2,
            title: 'Strategy',
            priceFormat: {
                type: 'price',
                precision: 2,
                minMove: 0.01,
            },
        });
        strategySeries.setData(data.strategy);

        // Benchmark line (gray, dashed)
        benchmarkSeries = chart.addSeries(LightweightCharts.LineSeries, {
            color: '#666666',
            lineWidth: 1,
            lineStyle: 2,
            title: 'Benchmark',
            priceFormat: {
                type: 'price',
                precision: 2,
                minMove: 0.01,
            },
        });
        benchmarkSeries.setData(data.benchmark);

        // Fit content with padding
        var dataLength = data.strategy.length;
        var rightPadding = Math.max(Math.ceil(dataLength * 0.2), 10);
        chart.timeScale().applyOptions({ rightOffset: rightPadding });
        chart.timeScale().fitContent();

        // Handle resize
        function handleResize() {
            chart.applyOptions({
                width: chartContainer.clientWidth,
            });
        }
        window.addEventListener('resize', handleResize);
        handleResize();

        // Set up fee toggle click handler
        var feeToggle = document.getElementById('fee-toggle');
        if (feeToggle) {
            feeToggle.addEventListener('click', toggleFees);
        }
    })
    .catch(function(error) {
        console.error('Error loading chart data:', error);
        document.getElementById('chart').innerHTML =
            '<p style="color: #666; text-align: center; padding: 2rem;">Failed to load chart data</p>';
    });

// Toggle between fee and fee-free views
function toggleFees() {
    if (!chartData) return;

    showingFees = !showingFees;
    var feeToggle = document.getElementById('fee-toggle');

    if (showingFees) {
        // Show with fees (normal view)
        strategySeries.setData(chartData.strategy);
        benchmarkSeries.setData(chartData.benchmark);
        updateMetricsDisplay(chartData.metrics, chartData.feesPaid, true);
        feeToggle.classList.remove('toggled');
    } else {
        // Show without fees
        strategySeries.setData(chartData.strategyNoFees);
        benchmarkSeries.setData(chartData.benchmarkNoFees);
        updateMetricsDisplay(chartData.metricsNoFees, 0, false);
        feeToggle.classList.add('toggled');
    }
}

// Update the metrics display
function updateMetricsDisplay(metrics, feesPaid, withFees) {
    // Update Total Return
    var returnEl = document.getElementById('metric-return');
    if (returnEl) {
        var returnVal = metrics.totalReturn;
        returnEl.textContent = (returnVal >= 0 ? '+' : '') + returnVal.toFixed(1) + '%';
        returnEl.className = 'value ' + (returnVal >= 0 ? 'positive' : 'negative');
    }

    // Update CAGR
    var cagrEl = document.getElementById('metric-cagr');
    if (cagrEl) {
        var cagrVal = metrics.cagr;
        cagrEl.textContent = cagrVal.toFixed(1) + '%';
        cagrEl.className = 'value ' + (cagrVal >= 0 ? 'positive' : 'negative');
    }

    // Update Sharpe
    var sharpeEl = document.getElementById('metric-sharpe');
    if (sharpeEl) {
        sharpeEl.textContent = metrics.sharpe.toFixed(2);
    }

    // Update Max Drawdown
    var maxddEl = document.getElementById('metric-maxdd');
    if (maxddEl) {
        maxddEl.textContent = '-' + metrics.maxDrawdown.toFixed(1) + '%';
    }

    // Update vs Benchmark
    var alphaEl = document.getElementById('metric-alpha');
    if (alphaEl) {
        var alpha = metrics.totalReturn - metrics.benchmarkReturn;
        alphaEl.textContent = (alpha >= 0 ? '+' : '') + alpha.toFixed(1) + '%';
        alphaEl.className = 'value ' + (alpha >= 0 ? 'positive' : 'negative');
    }

    // Update Fees Paid display
    var feesEl = document.getElementById('fee-value');
    if (feesEl) {
        if (withFees) {
            feesEl.textContent = '-$' + feesPaid.toLocaleString('en-US', {minimumFractionDigits: 2, maximumFractionDigits: 2});
        } else {
            feesEl.textContent = '$0.00';
        }
    }

    // Update fee toggle label
    var feeLabelEl = document.getElementById('fee-label');
    if (feeLabelEl) {
        feeLabelEl.textContent = withFees ? 'Fees Paid' : 'Fees Paid (OFF)';
    }
}
