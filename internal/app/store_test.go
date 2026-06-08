package app

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/store"
)

func swapActiveStore(t testing.TB, s store.Store) {
	t.Helper()
	previous := activeStore
	activeStore = s
	t.Cleanup(func() {
		activeStore = previous
	})
}

type memoryStore struct {
	files map[string][]byte
	dirs  map[string]struct{}
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		files: map[string][]byte{},
		dirs:  map[string]struct{}{},
	}
}

func (s *memoryStore) WriteBytes(path string, data []byte, _ fs.FileMode) error {
	s.mkdirParent(path)
	s.files[path] = append([]byte(nil), data...)
	return nil
}

func (s *memoryStore) ReadBytes(path string) ([]byte, error) {
	data, ok := s.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (s *memoryStore) AppendBytes(path string, data []byte) error {
	s.mkdirParent(path)
	s.files[path] = append(s.files[path], data...)
	return nil
}

func (s *memoryStore) Exists(path string) bool {
	_, ok := s.files[path]
	return ok
}

func (s *memoryStore) Mkdir(path string) error {
	if _, ok := s.files[path]; ok {
		return &fs.PathError{Op: "mkdir", Path: path, Err: fs.ErrExist}
	}
	if _, ok := s.dirs[path]; ok {
		return &fs.PathError{Op: "mkdir", Path: path, Err: fs.ErrExist}
	}
	parent := filepath.Dir(path)
	if parent != "." && parent != string(filepath.Separator) {
		if _, ok := s.dirs[parent]; !ok {
			return &fs.PathError{Op: "mkdir", Path: path, Err: fs.ErrNotExist}
		}
	}
	s.dirs[path] = struct{}{}
	return nil
}

func (s *memoryStore) MkdirAll(path string) error {
	for _, dir := range parentDirs(path) {
		s.dirs[dir] = struct{}{}
	}
	s.dirs[path] = struct{}{}
	return nil
}

func (s *memoryStore) ReadDir(path string) ([]os.DirEntry, error) {
	names := map[string]bool{}
	prefix := strings.TrimRight(path, string(filepath.Separator)) + string(filepath.Separator)
	for dir := range s.dirs {
		if dir == path || !strings.HasPrefix(dir, prefix) {
			continue
		}
		rest := strings.TrimPrefix(dir, prefix)
		if rest == "" || strings.Contains(rest, string(filepath.Separator)) {
			continue
		}
		names[rest] = true
	}
	for file := range s.files {
		if !strings.HasPrefix(file, prefix) {
			continue
		}
		rest := strings.TrimPrefix(file, prefix)
		if rest == "" || strings.Contains(rest, string(filepath.Separator)) {
			continue
		}
		names[rest] = false
	}
	if len(names) == 0 {
		if _, ok := s.dirs[path]; !ok {
			return nil, os.ErrNotExist
		}
	}
	entries := make([]os.DirEntry, 0, len(names))
	for name, isDir := range names {
		entries = append(entries, memoryDirEntry{name: name, isDir: isDir})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func (s *memoryStore) Open(path string) (io.ReadCloser, error) {
	data, ok := s.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *memoryStore) Remove(path string) error {
	if _, ok := s.files[path]; ok {
		delete(s.files, path)
		return nil
	}
	if _, ok := s.dirs[path]; ok {
		delete(s.dirs, path)
		return nil
	}
	return os.ErrNotExist
}

func (s *memoryStore) mkdirParent(path string) {
	_ = s.MkdirAll(filepath.Dir(path))
}

type memoryDirEntry struct {
	name  string
	isDir bool
}

func (e memoryDirEntry) Name() string { return e.name }
func (e memoryDirEntry) IsDir() bool  { return e.isDir }
func (e memoryDirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}
func (e memoryDirEntry) Info() (fs.FileInfo, error) { return memoryFileInfo(e), nil }

type memoryFileInfo memoryDirEntry

func (i memoryFileInfo) Name() string { return i.name }
func (i memoryFileInfo) Size() int64  { return 0 }
func (i memoryFileInfo) Mode() fs.FileMode {
	if i.isDir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (i memoryFileInfo) ModTime() time.Time { return time.Time{} }
func (i memoryFileInfo) IsDir() bool        { return i.isDir }
func (i memoryFileInfo) Sys() any           { return nil }

func parentDirs(path string) []string {
	dirs := []string{}
	for dir := filepath.Clean(path); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		dirs = append(dirs, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return dirs
}
