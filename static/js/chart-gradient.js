function registerGradientPlugin() {
    const inactiveStops = ['#FFFFFF']
    window.StreamGradientPlugin = {
        id: 'streamGradient',
        beforeDatasetsDraw: function (chart) {
            var area = chart.chartArea;
            if (!area) return;
            var ctx = chart.ctx;
            var meta = chart.$streamGradientMeta || {};
            var activeIndex = (chart.options.plugins && chart.options.plugins.streamGradient)
                ? chart.options.plugins.streamGradient.activeIndex : 0;
            var stopsByIndex = (chart.options.plugins && chart.options.plugins.streamGradient)
                ? chart.options.plugins.streamGradient.stopsByIndex : [];

            for (var i = 0; i < chart.data.datasets.length; i++) {
                var ds = chart.data.datasets[i];
                var stops = stopsByIndex[i] || [];

                // Add gradient to border colour and background color. Make inactive streams more transparent.
                var alpha = (i === activeIndex) ? 1 : 0.8;
                ds.borderColor = buildGradientWithAlpha(ctx, area, stops, alpha);

                var backgroundAlpha = (i === activeIndex) ? 0.8 : 0.5;
                var backgroundStops = (i === activeIndex) ? stops : inactiveStops;
                ds.backgroundColor = buildGradientWithAlpha(ctx, area, backgroundStops, backgroundAlpha);
            }
            chart.$streamGradientMeta = meta;
        }
    };

    // Auto-register if Chart is present
    if (window.Chart && window.Chart.register) {
        window.Chart.register(window.StreamGradientPlugin);
    }
}

function withAlpha(color, alpha) {
    if (typeof color === 'string' && color[0] === '#') {
        var r = parseInt(color.slice(1,3),16);
        var g = parseInt(color.slice(3,5),16);
        var b = parseInt(color.slice(5,7),16);
        return 'rgba(' + r + ',' + g + ',' + b + ',' + alpha + ')';
    }
    if (typeof color === 'string' && color.indexOf('rgb') === 0) {
        return color.replace(/rgba?\(([^)]+)\)/, function(_, inner){
            var p = inner.split(',').map(function(s){return s.trim();});
            return 'rgba(' + p[0] + ',' + p[1] + ',' + p[2] + ',' + alpha + ')';
        });
    }
    return color;
}

function buildGradient(ctx, area, stops) {
    var g = ctx.createLinearGradient(0, area.bottom, 0, area.top);
    // stops can be ["#..","#.."] or [{o:0,color:"#.."}, ...]
    if (!stops || !stops.length) return g;
    var i, n = stops.length;
    if (typeof stops[0] === 'string') {
        // even spacing
        for (i = 0; i < n; i++) {
            var off = (n === 1) ? 0 : (i / (n - 1));
            g.addColorStop(off, stops[i]);
        }
    } else {
        for (i = 0; i < n; i++) {
            var off2 = Math.max(0, Math.min(1, stops[i].o != null ? stops[i].o : stops[i].offset));
            var col2 = stops[i].c || stops[i].color;
            g.addColorStop(off2, col2);
        }
    }
    return g;
}

function buildGradientWithAlpha(ctx, area, stops, alpha) {
    // apply alpha to each stop color
    var arr, i, n;
    if (!stops || !stops.length) return buildGradient(ctx, area, stops);
    if (typeof stops[0] === 'string') {
        arr = [];
        n = stops.length;
        for (i = 0; i < n; i++) arr.push(withAlpha(stops[i], alpha));
        return buildGradient(ctx, area, arr);
    } else {
        arr = [];
        n = stops.length;
        for (i = 0; i < n; i++) {
            arr.push({ o: (stops[i].o != null ? stops[i].o : stops[i].offset), c: withAlpha(stops[i].c || stops[i].color, alpha) });
        }
        return buildGradient(ctx, area, arr);
    }
}