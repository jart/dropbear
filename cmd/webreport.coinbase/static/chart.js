// Initialize the chart when data is loaded
fetch('data.json')
    .then(function(response) {
        return response.json();
    })
    .then(function(data) {
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

        // Strategy line (cyan) - v5 API: addSeries(LineSeries, options)
        var strategySeries = chart.addSeries(LightweightCharts.LineSeries, {
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
        var benchmarkSeries = chart.addSeries(LightweightCharts.LineSeries, {
            color: '#666666',
            lineWidth: 1,
            lineStyle: 2, // dashed
            title: 'Benchmark',
            priceFormat: {
                type: 'price',
                precision: 2,
                minMove: 0.01,
            },
        });
        benchmarkSeries.setData(data.benchmark);

        // Fit content with 20% padding on the right for legend labels
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
    })
    .catch(function(error) {
        console.error('Error loading chart data:', error);
        document.getElementById('chart').innerHTML =
            '<p style="color: #666; text-align: center; padding: 2rem;">Failed to load chart data</p>';
    });
