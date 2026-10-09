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

    function showCount(cell, count, unitText, wanted) {
        cell.textContent = count.toLocaleString("en-US") + " ";
        const unit = document.createElement("span");
        unit.className = "unit";
        unit.textContent = unitText;
        cell.appendChild(unit);
        cell.classList.add(count >= wanted ? "stock-met" : "stock-short");
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
                            return;
                        }
                        showCount(cell, found.runs, "runs", wanted);
                        return;
                    }
                    showCount(cell, data.counts[cell.dataset.countKey] || 0, "units", wanted);
                });
                const notes = [];
                if (!data.characterAssets) notes.push("Your own assets couldn't be read from ESI, so they aren't counted.");
                if (!data.corpAssets) notes.push("Your corporation's assets couldn't be read (that needs the Director role), so only your own are counted.");
                if (anyRuns && !data.characterBlueprints) notes.push("Your own blueprints couldn't be read (that needs the esi-characters.read_blueprints.v1 scope), so their runs aren't counted.");
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
                });
            });
    }
})();
