package esi

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Images come from the EVE image server (https://images.evetech.net). Each
// one is downloaded once and kept on disk, so the image server is only asked
// about images this app hasn't seen before. Delete the folder to refetch them.
const imageDir = "data/icons"

// imageVariations lists, per image server category, the variations it serves.
var imageVariations = map[string][]string{
	"types":        {"icon", "render", "bp", "bpc"},
	"characters":   {"portrait"},
	"corporations": {"logo"},
	"alliances":    {"logo"},
}

// imageSizes are the sizes the image server accepts.
var imageSizes = []int{32, 64, 128, 256, 512, 1024}

// portraits and logos can change in game, so they are refetched after a day;
// type images never change.
const changeableImageTTL = 24 * time.Hour

var (
	// ErrImageNotFound means the image server has no image for that ID.
	ErrImageNotFound = errors.New("no image for this id")
	// ErrInvalidImage means the category, variation or size isn't one the
	// image server serves.
	ErrInvalidImage = errors.New("invalid image request")
)

var (
	imageLocks  sync.Map // cache file path -> *sync.Mutex, so one image downloads once
	imageMisses sync.Map // cache file path -> true, for IDs the image server lacks
)

// ImagePath returns the path of the cached image, downloading it first if
// needed, e.g. ImagePath("types", 34, "icon", 64) for Tritanium's icon or
// ImagePath("characters", id, "portrait", 128) for a character portrait.
func ImagePath(category string, id int64, variation string, size int) (string, error) {
	variations, ok := imageVariations[category]
	if !ok || !slices.Contains(variations, variation) || !slices.Contains(imageSizes, size) || id <= 0 {
		return "", ErrInvalidImage
	}

	dir := filepath.Join(imageDir, category)
	path := filepath.Join(dir, fmt.Sprintf("%d_%s_%d.png", id, variation, size))

	if isFresh(category, path) {
		return path, nil
	}
	if _, missing := imageMisses.Load(path); missing {
		return "", ErrImageNotFound
	}

	lock, _ := imageLocks.LoadOrStore(path, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	// another request may have downloaded it while this one waited
	if isFresh(category, path) {
		return path, nil
	}

	url := fmt.Sprintf("https://images.evetech.net/%s/%d/%s?size=%d", category, id, variation, size)
	if err := downloadImage(url, dir, path); err != nil {
		if errors.Is(err, ErrImageNotFound) {
			imageMisses.Store(path, true)
			return "", err
		}
		// keep serving a stale portrait or logo if the image server is down
		if _, statErr := os.Stat(path); statErr == nil {
			log.Printf("image %s: refetch failed, serving cached copy: %v", url, err)
			return path, nil
		}
		return "", err
	}
	return path, nil
}

func isFresh(category, path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if category == "types" {
		return true
	}
	return time.Since(info.ModTime()) < changeableImageTTL
}

func downloadImage(url, dir, path string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)

	log.Println("Querying:", url)

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return ErrImageNotFound
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return esiStatusError(url, response.Status, body)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// write to a temp file and rename, so a half-written image is never served
	tmp, err := os.CreateTemp(dir, "download-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
