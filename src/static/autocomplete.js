// Autocomplete for inputs with data-suggest="<url>". After 3 or more
// characters it fetches <url>?q=... (plus any extra params from the input's
// suggestParams function), which returns [{id, name, ...}], and lists the
// names under the input. Picking one fills the input and puts the id in the
// hidden input named by data-value-input; typing again clears it, so a form
// can only be sent with a picked suggestion.
(function () {
    const MIN_CHARS = 3;

    function attach(input) {
        const list = document.getElementById(input.getAttribute("aria-controls"));
        const valueInput = document.getElementById(input.dataset.valueInput);
        let results = [];
        let active = -1;
        let timer = null;
        let latest = 0;

        function close() {
            // Cancel pending lookups so they don't reopen the list.
            clearTimeout(timer);
            latest++;
            list.hidden = true;
            list.replaceChildren();
            input.setAttribute("aria-expanded", "false");
            input.removeAttribute("aria-activedescendant");
            results = [];
            active = -1;
        }

        function highlight(i) {
            active = i;
            list.querySelectorAll("li").forEach(function (li, j) {
                li.classList.toggle("active", j === i);
                li.setAttribute("aria-selected", j === i ? "true" : "false");
            });
            if (i >= 0) {
                input.setAttribute("aria-activedescendant", list.id + "-option-" + i);
            } else {
                input.removeAttribute("aria-activedescendant");
            }
        }

        function choose(i) {
            input.value = results[i].name;
            valueInput.value = results[i].id;
            input.setCustomValidity("");
            close();
        }

        function show(found) {
            results = found;
            list.replaceChildren();
            if (results.length === 0) {
                const li = document.createElement("li");
                li.className = "autocomplete-empty";
                li.textContent = "No matches";
                list.appendChild(li);
                list.hidden = false;
                return;
            }
            results.forEach(function (result, i) {
                const li = document.createElement("li");
                li.id = list.id + "-option-" + i;
                li.setAttribute("role", "option");
                const name = document.createElement("span");
                name.textContent = result.name;
                li.appendChild(name);
                const detail = [result.system, result.region].filter(Boolean).join(", ");
                if (detail) {
                    const small = document.createElement("span");
                    small.className = "autocomplete-detail";
                    small.textContent = detail;
                    li.appendChild(small);
                }
                // mousedown fires before the input's blur, so the click lands.
                li.addEventListener("mousedown", function (e) {
                    e.preventDefault();
                    choose(i);
                });
                li.addEventListener("mousemove", function () { highlight(i); });
                list.appendChild(li);
            });
            list.hidden = false;
            input.setAttribute("aria-expanded", "true");
            highlight(-1);
        }

        function lookup() {
            const q = input.value.trim();
            if (q.length < MIN_CHARS) {
                close();
                return;
            }
            const params = new URLSearchParams(input.suggestParams ? input.suggestParams() : {});
            params.set("q", q);
            const id = ++latest;
            fetch(input.dataset.suggest + "?" + params.toString())
                .then(function (r) { return r.ok ? r.json() : []; })
                .then(function (found) {
                    // Drop replies to older keystrokes that arrive late.
                    if (id === latest) show(found);
                })
                .catch(function () {});
        }

        input.addEventListener("input", function () {
            valueInput.value = "";
            clearTimeout(timer);
            timer = setTimeout(lookup, 150);
        });

        input.addEventListener("keydown", function (e) {
            if (list.hidden || results.length === 0) return;
            if (e.key === "ArrowDown") {
                e.preventDefault();
                highlight((active + 1) % results.length);
            } else if (e.key === "ArrowUp") {
                e.preventDefault();
                highlight(active <= 0 ? results.length - 1 : active - 1);
            } else if (e.key === "Enter" && active >= 0) {
                // Pick the suggestion instead of submitting the form.
                e.preventDefault();
                choose(active);
            } else if (e.key === "Escape") {
                // Close the list without closing the dialog it's in.
                e.preventDefault();
                close();
            }
        });

        input.addEventListener("blur", close);

        // Only let the form send a picked suggestion.
        input.form.addEventListener("submit", function (e) {
            if (!valueInput.value) {
                e.preventDefault();
                input.setCustomValidity("Pick one of the suggestions.");
                input.reportValidity();
            }
        });
        input.addEventListener("input", function () { input.setCustomValidity(""); });

        // Lets other scripts clear the field, e.g. when a dialog reopens.
        input.resetSuggestion = function () {
            input.value = "";
            valueInput.value = "";
            input.setCustomValidity("");
            close();
        };
    }

    document.querySelectorAll("input[data-suggest]").forEach(attach);
})();
