// Package storage implementa ports.MediaReader sobre o filesystem local (./media).
package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// ErrInvalidKey indica chave que escaparia do diretório de mídia.
var ErrInvalidKey = errors.New("chave de mídia inválida")

type LocalFS struct {
	root string
}

var _ ports.MediaReader = (*LocalFS)(nil)

func NewLocalFS(root string) (*LocalFS, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &LocalFS{root: abs}, nil
}

// Path resolve a chave dentro da raiz, bloqueando path traversal e arquivos parciais.
func (l *LocalFS) Path(key string) (string, error) {
	if key == "" || strings.Contains(key, "\\") || strings.Contains(key, ".partial-") {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	full := filepath.Join(l.root, filepath.FromSlash(key))
	if !strings.HasPrefix(full, l.root+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return full, nil
}

func (l *LocalFS) Open(_ context.Context, key string) (ports.MediaFile, error) {
	path, err := l.Path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: mídia %s", domain.ErrNotFound, key)
	}
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("%w: %q não é arquivo", ErrInvalidKey, key)
	}
	return &file{File: f, size: info.Size(), mod: info.ModTime()}, nil
}

type file struct {
	*os.File
	size int64
	mod  time.Time
}

func (f *file) Size() int64        { return f.size }
func (f *file) ModTime() time.Time { return f.mod }

// Entry é um arquivo no diretório de mídia (para a limpeza).
type Entry struct {
	Key     string // caminho relativo com "/" (ex.: job/render.mp4)
	ModTime time.Time
	Size    int64
	Partial bool // arquivo temporário de um render em andamento ou abortado
}

// List percorre o diretório de mídia (sem seguir links simbólicos).
func (l *LocalFS) List(ctx context.Context) ([]Entry, error) {
	var out []Entry
	err := filepath.WalkDir(l.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path == l.root {
				return fs.SkipAll
			}
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(l.root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		out = append(out, Entry{Key: key, ModTime: info.ModTime(), Size: info.Size(), Partial: strings.Contains(key, ".partial-")})
		return nil
	})
	return out, err
}

// Remove apaga um arquivo (inclusive parciais) e o diretório do job se ficar vazio.
func (l *LocalFS) Remove(_ context.Context, key string) error {
	full := filepath.Join(l.root, filepath.FromSlash(key))
	if key == "" || !strings.HasPrefix(full, l.root+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if dir := filepath.Dir(full); dir != l.root {
		_ = os.Remove(dir) // só remove se estiver vazio
	}
	return nil
}
