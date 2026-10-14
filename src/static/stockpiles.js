// Stockpiles page: opens the New Stockpile and Add Item dialogs, and switches
// the Add Item dialog between items and blueprint runs.
(function () {
    const addItemTarget = document.getElementById("add-item-stockpile");
    const addItemStockpileId = document.getElementById("add-item-stockpile-id");
    const typeInput = document.getElementById("add-item-type");
    const typeLabel = document.getElementById("add-item-type-label");
    const quantityInput = document.getElementById("add-item-quantity");
    const quantityLabel = document.getElementById("add-item-quantity-label");
    const runsChoice = document.getElementById("kind-runs");

    function updateKind() {
        const runs = runsChoice.checked;
        typeLabel.textContent = runs ? "Blueprint" : "Item";
        typeInput.placeholder = runs
            ? "Type 3 or more letters of the blueprint name"
            : "Type 3 or more letters of the item name";
        quantityLabel.textContent = runs ? "Runs wanted" : "Quantity wanted";
    }

    // Blueprint Runs only suggests blueprints.
    typeInput.suggestParams = function () {
        return { kind: document.querySelector("input[name=kind]:checked").value };
    };

    document.querySelectorAll("input[name=kind]").forEach(function (radio) {
        radio.addEventListener("change", function () {
            typeInput.resetSuggestion();
            updateKind();
            typeInput.focus();
        });
    });

    document.querySelectorAll("[data-open-dialog]").forEach(function (button) {
        button.addEventListener("click", function () {
            const dialog = document.getElementById(button.dataset.openDialog);
            const form = dialog.querySelector("form");
            form.reset();
            dialog.querySelectorAll("input[data-suggest]").forEach(function (input) {
                input.resetSuggestion();
            });
            if (dialog.id === "add-item") {
                addItemTarget.textContent = button.dataset.stockpileName;
                addItemStockpileId.value = button.dataset.stockpileId;
                updateKind();
            }
            dialog.showModal();
        });
    });

    document.querySelectorAll("[data-close-dialog]").forEach(function (button) {
        button.addEventListener("click", function () {
            button.closest("dialog").close();
        });
    });

    function withUnit(cell, n, unitText) {
        cell.textContent = n.toLocaleString("en-US") + " ";
        const unit = document.createElement("span");
        unit.className = "unit";
        unit.textContent = unitText;
        cell.appendChild(unit);
    }

    function showCount(cell, count, unitText, wanted) {
        withUnit(cell, count, unitText);
        cell.dataset.sortValue = count;
        cell.classList.add(count >= wanted ? "stock-met" : "stock-short");
        showProgress(cell, Math.max(wanted - count, 0), unitText, wanted > 0 ? count / wanted : 1);
    }

    // 1234567890 -> "1.23b"
    function shortIsk(v) {
        const units = [[1e12, "t"], [1e9, "b"], [1e6, "m"], [1e3, "k"]];
        for (const [size, suffix] of units) {
            if (Math.abs(v) >= size) return (v / size).toFixed(2) + suffix;
        }
        return v.toFixed(2);
    }

    // Shows an ISK value, or a dash with emptyTitle when value is null.
    function showValue(cell, value, emptyTitle) {
        if (value === null) {
            const dash = document.createElement("span");
            dash.className = "muted";
            dash.textContent = "—";
            cell.replaceChildren(dash);
            cell.title = emptyTitle;
            cell.dataset.sortValue = -1; // below a value of 0
            return;
        }
        cell.textContent = shortIsk(value) + " ";
        const unit = document.createElement("span");
        unit.className = "unit";
        unit.textContent = "ISK";
        cell.appendChild(unit);
        cell.title = value.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + " ISK";
        cell.dataset.sortValue = value;
    }

    // Fills in the Stockpile Value and Missing Value cells at the Jita sell
    // price, or dashes when there's no price.
    function showValues(row, count, missing, price, emptyTitle) {
        const priced = typeof price === "number";
        showValue(row.querySelector(".stock-value"), priced ? count * price : null, emptyTitle);
        showValue(row.querySelector(".missing-value"), priced ? missing * price : null, emptyTitle);
    }

    // Fills in the Missing and Level cells next to an In Station cell.
    // fraction is how much of Wanted is in station; the bar stops at full,
    // but the percent shows any extra, e.g. 130%.
    function showProgress(cell, missing, unitText, fraction) {
        const row = cell.closest("tr");
        const missingCell = row.querySelector(".missing-cell");
        withUnit(missingCell, missing, unitText);
        missingCell.dataset.sortValue = missing;
        missingCell.classList.toggle("stock-short", missing > 0);
        missingCell.classList.toggle("muted", missing === 0);

        const level = row.querySelector(".level");
        level.parentElement.dataset.sortValue = fraction;
        const filled = Math.min(fraction, 1);
        const fill = level.querySelector(".level-fill");
        fill.style.width = (filled * 100) + "%";
        // red at empty, through amber, to green at full
        fill.style.backgroundColor = "hsl(" + Math.round(filled * 120) + ", 70%, 45%)";
        level.querySelector(".level-percent").textContent = Math.floor(fraction * 100) + "%";
        level.hidden = false;
    }

    // Fill in the In Station column. Counts come from ESI assets, which can
    // take a few seconds the first time, so the page shows without them.
    const countCells = document.querySelectorAll("[data-count-key]");
    if (countCells.length > 0) {
        fetch("/stockpiles/counts")
            .then(function (r) {
                if (!r.ok) throw new Error(r.status);
                return r.json();
            })
            .then(function (data) {
                let anyRuns = false;
                countCells.forEach(function (cell) {
                    const wanted = Number(cell.dataset.wanted);
                    if (cell.hasAttribute("data-runs")) {
                        anyRuns = true;
                        const found = data.runs[cell.dataset.countKey] || { runs: 0, bpo: false };
                        if (found.bpo) {
                            // an original makes as many runs as wanted
                            cell.textContent = "BPO";
                            cell.title = found.runs > 0
                                ? "An original is here, plus " + found.runs.toLocaleString("en-US") + " runs on copies"
                                : "An original is here";
                            cell.classList.add("stock-met");
                            // an original outranks any number of runs
                            cell.dataset.sortValue = Number.MAX_SAFE_INTEGER;
                            showProgress(cell, 0, "runs", 1);
                            showValues(cell.closest("tr"), 0, 0, null, "Blueprint runs aren't valued");
                            return;
                        }
                        showCount(cell, found.runs, "runs", wanted);
                        showValues(cell.closest("tr"), 0, 0, null, "Blueprint runs aren't valued");
                        return;
                    }
                    const count = data.counts[cell.dataset.countKey] || 0;
                    showCount(cell, count, "units", wanted);
                    const typeId = cell.dataset.countKey.split("-")[1];
                    showValues(cell.closest("tr"), count, Math.max(wanted - count, 0), (data.prices || {})[typeId],
                        "No Jita sell price for this item");
                });
                const notes = [];
                if (!data.characterAssets) notes.push("Your own assets couldn't be read from ESI, so they aren't counted.");
                if (!data.corpAssets) notes.push("Your corporation's assets couldn't be read (that needs the Director role), so only your own are counted.");
                if (anyRuns && !data.characterBlueprints) notes.push("Your own blueprints couldn't be read (that needs the esi-characters.read_blueprints.v1 scope), so their runs aren't counted.");
                if (data.pricesStatus === "fetching") notes.push("Jita prices are being fetched now, so some values are missing. Reload in a minute or two.");
                if (data.pricesStatus === "unavailable") notes.push("Values need Jita prices, which need the net worth tables in the database (see the server log).");
                if (data.pricesStatus === "error") notes.push("Jita prices couldn't be read from the database, so values are missing.");
                if (anyRuns && !data.corpBlueprints) notes.push("Your corporation's blueprints couldn't be read (that needs the esi-corporations.read_blueprints.v1 scope and the Director role), so their runs aren't counted.");
                document.querySelectorAll(".count-note").forEach(function (note) {
                    note.textContent = notes.join(" ");
                    note.hidden = notes.length === 0;
                });
            })
            .catch(function () {
                countCells.forEach(function (cell) {
                    const unavailable = document.createElement("span");
                    unavailable.className = "muted";
                    unavailable.title = "Couldn't read assets from ESI. Reload to try again.";
                    unavailable.textContent = "Unavailable";
                    cell.replaceChildren(unavailable);
                    cell.closest("tr").querySelectorAll(".missing-cell, .value-cell").forEach(function (c) {
                        c.textContent = "";
                    });
                });
            });
    }

    // Click a column heading to sort that stockpile's table by it: ascending
    // first, then descending on the next click. Tables start sorted by item
    // name, which is how the server lists them. Rows still loading their
    // counts sort as if empty.
    const collator = new Intl.Collator("en", { numeric: true, sensitivity: "base" });

    function sortValue(row, column, type) {
        const cell = row.cells[column];
        const raw = cell.dataset.sortValue !== undefined ? cell.dataset.sortValue : cell.textContent.trim();
        return type === "number" ? (Number(raw) || 0) : raw;
    }

    document.querySelectorAll("table.sortable").forEach(function (table) {
        const headers = table.querySelectorAll("th[data-sort]");
        headers.forEach(function (th) {
            // a button inside the heading makes it reachable by keyboard
            const button = document.createElement("button");
            button.type = "button";
            button.className = "sort-button";
            button.append(...th.childNodes);
            th.appendChild(button);

            button.addEventListener("click", function () {
                const ascending = th.getAttribute("aria-sort") !== "ascending";
                headers.forEach(function (other) { other.removeAttribute("aria-sort"); });
                th.setAttribute("aria-sort", ascending ? "ascending" : "descending");

                const column = th.cellIndex;
                const type = th.dataset.sort;
                const body = table.tBodies[0];
                const rows = Array.from(body.rows).filter(function (row) { return !row.querySelector("td.empty"); });
                rows.sort(function (a, b) {
                    const x = sortValue(a, column, type);
                    const y = sortValue(b, column, type);
                    let order = type === "number" ? x - y : collator.compare(x, y);
                    if (order === 0) {
                        // ties keep a steady order, by item name
                        order = collator.compare(sortValue(a, 0, "text"), sortValue(b, 0, "text"));
                        return order;
                    }
                    return ascending ? order : -order;
                });
                rows.forEach(function (row) { body.appendChild(row); });
            });
        });
    });
})();
