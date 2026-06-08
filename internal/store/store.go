package store

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type Store interface {
	WriteBytes(path string, data []byte, perm fs.FileMode) error
	ReadBytes(path string) ([]byte, error)
	AppendBytes(path string, data []byte) error
	Exists(path string) bool
	Mkdir(path string) error
	MkdirAll(path string) error
	ReadDir(path string) ([]os.DirEntry, error)
	Open(path string) (io.ReadCloser, error)
	Remove(path string) error
}

type FS struct{}

func (FS) WriteBytes(path string, data []byte, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

func (FS) ReadBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (FS) AppendBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}

func (FS) Exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (FS) Mkdir(path string) error {
	return os.Mkdir(path, 0o755)
}

func (FS) MkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}

func (FS) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

func (FS) Open(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (FS) Remove(path string) error {
	return os.Remove(path)
}
