// Draws the Net Worth chart from the points the page embeds, and fills in
// the headline numbers from the newest point.
(function () {
    const pointsEl = document.getElementById("networth-points");
    const points = pointsEl ? JSON.parse(pointsEl.textContent || "[]") || [] : [];

    // 1234567890 -> "1.23b ISK"
    function shortIsk(v) {
        const abs = Math.abs(v);
        const units = [[1e12, "t"], [1e9, "b"], [1e6, "m"], [1e3, "k"]];
        for (const [size, suffix] of units) {
            if (abs >= size) return (v / size).toFixed(2) + suffix + " ISK";
        }
        return v.toFixed(0) + " ISK";
    }

    function fullIsk(v) {
        return v.toLocaleString(undefined, {minimumFractionDigits: 2, maximumFractionDigits: 2}) + " ISK";
    }

    const latest = points[points.length - 1];
    if (latest) {
        for (const [id, value] of [["stat-net", latest.netWorth], ["stat-wallet", latest.wallet], ["stat-assets", latest.assetValue]]) {
            const el = document.getElementById(id);
            el.textContent = shortIsk(value);
            el.title = fullIsk(value);
        }
    }

    const canvas = document.getElementById("networth-chart");
    if (!canvas || !window.Chart || points.length === 0) return;

    const css = getComputedStyle(document.documentElement);
    const token = (name) => css.getPropertyValue(name).trim();
    const times = points.map((p) => Date.parse(p.time));

    function line(label, key, color, width) {
        return {
            label,
            data: points.map((p, i) => ({x: times[i], y: p[key]})),
            borderColor: color,
            backgroundColor: color,
            borderWidth: width,
            pointRadius: points.length > 48 ? 0 : 2,
            pointHoverRadius: 4,
            tension: 0,
        };
    }

    const dateFormat = new Intl.DateTimeFormat(undefined, {day: "numeric", month: "short"});
    const dateTimeFormat = new Intl.DateTimeFormat(undefined, {day: "numeric", month: "short", hour: "2-digit", minute: "2-digit"});
    const span = times[times.length - 1] - times[0];

    new Chart(canvas, {
        type: "line",
        data: {
            datasets: [
                line("Net worth", "netWorth", token("--accent"), 3),
                line("Wallets", "wallet", token("--good"), 2),
                line("Assets", "assetValue", token("--pending"), 2),
            ],
        },
        options: {
            maintainAspectRatio: false,
            animation: false,
            interaction: {mode: "index", intersect: false},
            color: token("--muted"),
            scales: {
                x: {
                    type: "linear",
                    min: times[0],
                    max: times[times.length - 1],
                    grid: {display: false},
                    ticks: {
                        color: token("--muted"),
                        maxTicksLimit: 8,
                        callback: (v) => (span > 2 * 86400e3 ? dateFormat : dateTimeFormat).format(v),
                    },
                },
                y: {
                    grid: {color: token("--line")},
                    ticks: {color: token("--muted"), callback: (v) => shortIsk(v)},
                },
            },
            plugins: {
                legend: {position: "bottom", labels: {color: token("--text"), boxWidth: 12, boxHeight: 12}},
                tooltip: {
                    callbacks: {
                        title: (items) => dateTimeFormat.format(items[0].parsed.x),
                        label: (item) => item.dataset.label + ": " + fullIsk(item.parsed.y),
                    },
                },
            },
        },
    });
})();
