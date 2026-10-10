package main

import (
	"Stockpile-Manager/src/auth"
	"Stockpile-Manager/src/esi"
	"Stockpile-Manager/src/sde"
	"errors"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

var tmpl = template.Must(
	template.New("").Funcs(template.FuncMap{
		"commaInt": commaInt,
	}).ParseGlob("src/templates/*.html"),
)

// commaInt formats n with thousands separators, e.g. 1200 -> "1,200".
func commaInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")

	var result []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, c)
	}
	if neg {
		return "-" + string(result)
	}
	return string(result)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.ClearSessionCookie(w, r)
	http.Redirect(w, r, "/", http.StatusFound)
}

func main() {
	// a missing or half-filled config/.env still starts the site, without sign-in
	if err := auth.InitAuth(); err != nil {
		log.Printf("EVE sign-in disabled: %v", err)
	}
	initStockpiles()
	initNetWorth()
	auth.OnSignIn = refreshStructuresOnSignIn
	// the SDE takes a few seconds to read, so start before anyone searches
	go func() {
		if err := sde.Load(); err != nil {
			log.Printf("item search disabled: %v", err)
		}
	}()

	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)
	staticDir := filepath.Join(baseDir, "static")

	fs := http.FileServer(http.Dir(staticDir))
	// no-cache makes the browser check for a newer file on every page load
	// (a cheap 304 when it hasn't changed), so updated scripts and styles are
	// picked up without a hard refresh
	http.Handle("/static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fs.ServeHTTP(w, r)
	})))

	http.HandleFunc("/", renderBase)
	http.HandleFunc("/image", handleImage)
	http.HandleFunc("/stockpiles", auth.RequireAuth(renderStockpiles))
	http.HandleFunc("/stockpiles/create", auth.RequireAuth(handleCreateStockpile))
	http.HandleFunc("/stockpiles/delete", auth.RequireAuth(handleDeleteStockpile))
	http.HandleFunc("/stockpiles/items/add", auth.RequireAuth(handleAddStockpileItem))
	http.HandleFunc("/stockpiles/items/remove", auth.RequireAuth(handleRemoveStockpileItem))
	http.HandleFunc("/stockpiles/structures/refresh", auth.RequireAuth(handleRefreshStructures))
	http.HandleFunc("/stockpiles/counts", auth.RequireAuth(handleStockpileCounts))
	http.HandleFunc("/stockpiles/item-suggestions", auth.RequireAuth(handleItemSuggestions))
	http.HandleFunc("/stockpiles/structure-suggestions", auth.RequireAuth(handleStructureSuggestions))
	http.HandleFunc("/networth", auth.RequireAuth(renderNetWorth))
	http.HandleFunc("/networth/snapshot", auth.RequireAuth(handleNetWorthSnapshot))
	http.HandleFunc("/api/assets", auth.RequireAuth(handleAssets))
	http.HandleFunc("/api/structures", auth.RequireAuth(handleStructures))

	http.HandleFunc("/auth/login", auth.HandleLogin)
	http.HandleFunc("/auth/callback", auth.HandleCallback)
	http.HandleFunc("/auth/logout", handleLogout)

	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "5002"
	}

	log.Printf("Server listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func renderBase(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	sess, loggedIn := auth.CurrentSession(r)

	data := struct {
		LoggedIn      bool
		SSOConfigured bool
		CharacterID   int64
		CharacterName string
	}{LoggedIn: loggedIn, SSOConfigured: auth.Configured()}
	if loggedIn {
		data.CharacterID = sess.CharacterID
		data.CharacterName = sess.CharacterName
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		log.Printf("renderBase: %v", err)
	}
}

// handleImage serves an image from the EVE image server through the local
// cache, downloading it the first time.
// ?category=types|characters|corporations|alliances&id=<ID>&variation=<v>&size=<px>
// e.g. /image?category=types&id=34&variation=icon&size=64 for Tritanium.
// variation defaults to the category's usual one and size to 64.
func handleImage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	category := q.Get("category")
	id, err := strconv.ParseInt(q.Get("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	variation := q.Get("variation")
	if variation == "" {
		variation = defaultImageVariation[category]
	}
	size := 64
	if s := q.Get("size"); s != "" {
		if size, err = strconv.Atoi(s); err != nil {
			http.Error(w, "invalid size", http.StatusBadRequest)
			return
		}
	}

	path, err := esi.ImagePath(category, id, variation, size)
	switch {
	case errors.Is(err, esi.ErrInvalidImage):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case errors.Is(err, esi.ErrImageNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		log.Printf("image %s %d: %v", category, id, err)
		http.Error(w, "could not load image", http.StatusBadGateway)
		return
	}

	// let the browser keep images for a day; the cache refetches portraits daily
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, path)
}

var defaultImageVariation = map[string]string{
	"types":        "icon",
	"characters":   "portrait",
	"corporations": "logo",
	"alliances":    "logo",
}
