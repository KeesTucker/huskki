window.onServerTime = (ms) => {
    console.log("server time (ms):", ms);
};

// Allows data to be pushed into a local buffer on the page for storing timeseries
// data before it is consumed by a chart.
function pushData(chartKey, streamKey, x, y) {
    if (!window[streamKey + 'Buffer']) window[streamKey + 'Buffer'] = [];
    window[streamKey + 'Buffer'].push({ x, y });
}

// Cycles the active stream for the chart matching chartKey to the stream selected with active (index)
function cycleStream(chartKey, active){
    const chart = Chart.getChart(chartKey + '-chart');
    if (!chart) return;

    var s = chart.options.plugins.streaming || (chart.options.plugins.streaming = {});
    var g = (chart.options.plugins.streamGradient || (chart.options.plugins.streamGradient = {}));
    var prevPause = !!s.pause;
    s.pause = true;
    try {
        g.activeIndex = active; // tell the gradient plugin which dataset is active
        chart.data.datasets.forEach(function(ds, i){
            ds.borderWidth = (i === active) ? 3 : 0;
        });
        chart.update('none');
    } finally {
        s.pause = prevPause;
        chart.update('none');
    }
}