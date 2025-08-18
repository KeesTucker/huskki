function onChartRefresh(chart, metaByIndex) {
    const currentTime = getTime();
    const MAX_POINTS = 2000;
    const ALPHA = 0.5;   // EMA smoothing for non-discrete
    const FILL_MS = 100; // discrete hold interval

    chart.data.datasets.forEach((ds, i) => {
        const meta = metaByIndex[i] || {};
        const buff = window[meta.key + 'Buffer'] || [];

        // Ensure per-dataset state fields exist
        if (ds._nextFillX === undefined) ds._nextFillX = null;
        if (ds._discreteActive === undefined) ds._discreteActive = false;
        if (ds._lastDiscreteY === undefined) ds._lastDiscreteY = undefined;
        if (ds._ema === undefined) ds._ema = undefined;
        if (ds._sentinel === undefined) ds._sentinel = null;

        // Drain this stream's buffer
        while (buff.length) {
            const raw = buff.shift(); // { x, y } where x is ms epoch
            const insertIdx = ds._sentinel ? ds.data.length - 1 : ds.data.length;

            if (meta.discrete) {
                // Discrete: no smoothing, hold-last behavior handled below
                ds.data.splice(insertIdx, 0, { x: raw.x, y: raw.y });
                ds._ema = undefined;
                ds._nextFillX = Math.max(ds._nextFillX ?? (raw.x + FILL_MS), raw.x + FILL_MS);
                ds._discreteActive = true;
                ds._lastDiscreteY = raw.y;
            } else if (meta.smoothing) {
                // Continuous: EMA smoothing
                ds._ema = (ds._ema == null) ? raw.y : (ALPHA * raw.y + (1 - ALPHA) * ds._ema);
                ds.data.splice(insertIdx, 0, { x: raw.x, y: ds._ema });
                ds._discreteActive = false;
                ds._nextFillX = null;
                ds._lastDiscreteY = undefined;
            } else {
                // Raw representation
                ds.data.splice(insertIdx, 0, { x: raw.x, y: raw.y });
                ds._discreteActive = false;
                ds._nextFillX = null;
                ds._lastDiscreteY = undefined;
            }
        }

        // If in discrete mode, fill every FILL_MS with last Y up to now
        const realCount = ds.data.length - (ds._sentinel ? 1 : 0);
        const lastReal = realCount > 0 ? ds.data[realCount - 1] : null;

        if (ds._discreteActive && lastReal) {
            ds._lastDiscreteY = lastReal.y;
            if (ds._nextFillX == null) ds._nextFillX = lastReal.x + FILL_MS;

            let insertIdx = ds._sentinel ? ds.data.length - 1 : ds.data.length;
            while (ds._nextFillX <= currentTime) {
                ds.data.splice(insertIdx, 0, { x: ds._nextFillX, y: ds._lastDiscreteY });
                insertIdx++;
                ds._nextFillX += FILL_MS;
            }
        }

        // Maintain a single sentinel pinned to the right edge, matching last real Y
        if (lastReal) {
            if (!ds._sentinel) {
                const s = { x: currentTime, y: lastReal.y, _sentinel: true };
                ds.data.push(s);
                ds._sentinel = s;
            } else {
                ds._sentinel.x = currentTime;
                ds._sentinel.y = lastReal.y;
            }
        } else if (ds._sentinel) {
            ds.data.pop();
            ds._sentinel = null;
        }

        // Trim old real points but never drop the sentinel
        const over = (ds.data.length - (ds._sentinel ? 1 : 0)) - MAX_POINTS;
        if (over > 0) ds.data.splice(0, over);
    });
}