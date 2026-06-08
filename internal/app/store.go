package app

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/heurema/pactum/internal/store"
)

var activeStore store.Store = store.FS{}

func storeDirExists(path string) (bool, error) {
	if activeStore.Exists(path) {
		return false, nil
	}
	if _, err := activeStore.ReadDir(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func storeFileSHA256(path string) (string, error) {
	file, err := activeStore.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
