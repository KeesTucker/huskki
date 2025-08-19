window.onServerTime = (serverTime) => {
    // slop moves the stream forward a small amount so we don't add points in the future due slight server time
    // inconsistency from latency etc (causes graphical flashing artifacts)
    const slop = 50 // TODO: probably replay or augment this with a ping latency calculation
    const receivedAt = Date.now() - slop;
    window.timeOffset = serverTime - receivedAt;
};

function getTime() {
    // TODO: in the future it would be cool to check a mode var (replay/realtime) to see if we should use this
    //  implementation (client time + sync) or switch to pure server time. This would allow cool things
    //  like scrubbing through time.
    return Date.now() + (window.timeOffset || 0);
}

// Allows data to be pushed into a local buffer on the page for storing timeseries
// data before it is consumed by a stream.
function pushData(chartKey, streamKey, x, y) {
    if (!window[streamKey + 'Buffer']) window[streamKey + 'Buffer'] = [];
    window[streamKey + 'Buffer'].push({ x, y });
}

// Cycles the active stream for the stream matching chartKey to the stream selected with active (index)
function cycleStream(chartKey, active){
    const chart = Chart.getChart(chartKey + '-stream');
    if (!chart) return;

    var s = chart.options.plugins.streaming || (chart.options.plugins.streaming = {});
    var g = (chart.options.plugins.streamGradient || (chart.options.plugins.streamGradient = {}));
    var prevPause = !!s.pause;
    s.pause = true;
    try {
        g.activeIndex = active; // tell the gradient plugin which dataset is active
        chart.data.datasets.forEach(function(ds, i){
            ds.borderWidth = (i === active) ? 4 : 2;
        });
        chart.update('none');
    } finally {
        s.pause = prevPause;
        chart.update('none');
    }
}