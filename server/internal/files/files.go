// Package files хранит загруженные фото на диске.
package files

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// AllowedTypes — допустимые типы фото и расширения для них.
var AllowedTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

var (
	ErrUnsupportedType = errors.New("unsupported file type")
	ErrEmpty           = errors.New("empty file")
)

type Storage struct {
	dir string
}

func NewStorage(dir string) (*Storage, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return &Storage{dir: dir}, nil
}

type Saved struct {
	Key         string
	ContentType string
	Size        int64
}

// Save определяет тип по содержимому (а не по заголовкам клиента),
// сохраняет файл под случайным именем и возвращает ключ хранения.
func (s *Storage) Save(r io.Reader) (Saved, error) {
	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return Saved{}, err
	}
	head = head[:n]
	if n == 0 {
		return Saved{}, ErrEmpty
	}
	contentType := http.DetectContentType(head)
	ext, ok := AllowedTypes[contentType]
	if !ok {
		return Saved{}, ErrUnsupportedType
	}

	key := filepath.Join(time.Now().Format("2006/01"), randomName()+ext)
	path := filepath.Join(s.dir, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return Saved{}, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return Saved{}, err
	}
	size, err := io.Copy(f, io.MultiReader(bytes.NewReader(head), r))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return Saved{}, fmt.Errorf("write %s: %w", key, err)
	}
	return Saved{Key: filepath.ToSlash(key), ContentType: contentType, Size: size}, nil
}

func (s *Storage) Open(key string) (*os.File, error) {
	return os.Open(filepath.Join(s.dir, filepath.FromSlash(key)))
}

func (s *Storage) Remove(key string) {
	os.Remove(filepath.Join(s.dir, filepath.FromSlash(key)))
}

func randomName() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
