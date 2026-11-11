// Package paste stores images pasted into the composer, so their paths
// can be sent to an agent, which reads them as images.
package paste

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var types = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// MaxBytes caps one pasted image.
const MaxBytes = 10 << 20

var ErrNotImage = errors.New("only PNG, JPEG, GIF and WebP images can be pasted")

type Store struct {
	// Dir holds the images, owner-only.
	Dir string
}

// Save writes an image and returns its path. The type comes from the
// bytes, not from what the browser claims.
func (s Store) Save(data []byte) (string, error) {
	if len(data) == 0 || len(data) > MaxBytes {
		return "", errors.New("image is empty or over 10 MB")
	}
	ext, ok := types[http.DetectContentType(data)]
	if !ok {
		return "", ErrNotImage
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(s.Dir, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(s.Dir, "paste-*"+ext)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), f.Close()
}

// Cleanup removes pasted images older than age, so agents can still open
// a recent one while the directory stays small.
func (s Store) Cleanup(age time.Duration, now time.Time) int {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return 0
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "paste-") {
			continue
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) < age {
			continue
		}
		if os.Remove(filepath.Join(s.Dir, e.Name())) == nil {
			removed++
		}
	}
	return removed
}
