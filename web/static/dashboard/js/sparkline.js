function s(key, leftX, rightX, points) {
    // Select the svg
    const svg = document.querySelector(`#stream-${key}-line svg`);
    if (!svg) {
        return;
    }
    // Update the view box with the new leftX timestamp so the sparkline scrolls nicely
    const viewBox = svg.getAttribute('viewBox')
    const viewBoxParams = viewBox.split(" ");
    viewBoxParams[0] = leftX;
    svg.setAttribute('viewBox', viewBoxParams.join(" "));

    if (!points) {
        return;
    }

    // Select the polyline for this stream key
    const poly = document.querySelector(`#polyline-${key}`);
    if (!poly) return;

    // Get current points, or start fresh
    let pts = poly.getAttribute("points").trim();
    // TODO: This is slow, should keep a running array of points so we don't need to keep adding parsing them from the svg.
    let arr = pts ? pts.split(" ") : [];

    // remove old sentinel
    if (arr.length) {
        arr.pop();
    }

    // Clean up any points not in/effecting the sparkline within the view window so we don't build an infinitely long svg
    let removeBefore;
    for (let i = 0, len = arr.length; i < len; i++) {
        let point = arr[i];
        const x = Number(point.split(",")[0]);
        if (x < leftX && i > 1) {
            removeBefore = i - 1
        }
    }
    arr.splice(0, removeBefore);

    // Clean up any points with bad timestamps in the future
    arr = arr.filter(p => {
        const x = Number(p.split(",")[0]);
        return x <= rightX;
    });

    // Append new points from the map
    for (const x in points) {
        const y = points[x];
        arr.push(`${x},${y}`);
    }

    // Add sentinel at rightX with the last Y value
    if (arr.length > 0) {
        const last = arr[arr.length - 1].split(",");
        const lastY = last[1];
        arr.push(`${rightX},${lastY}`);
    }

    // Update the polyline
    poly.setAttribute("points", arr.join(" "));
}

function b(chartKey, activeStreamKey) {
    const card = document.getElementById(`${chartKey}-card`);
    if (!card) return;

    // Set all polylines in this chart to 3
    const polys = card.querySelectorAll('.chart polyline[id^="polyline-"]');
    polys.forEach(pl => pl.setAttribute('stroke-width', '3'));

    // Now set the active stream's polyline to 5
    if (activeStreamKey != null) {
        const active = card.querySelector(`[id="polyline-${activeStreamKey}"]`);
        if (active) active.setAttribute('stroke-width', '5');
    }
}
