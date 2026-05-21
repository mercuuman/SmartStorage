package files

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Storage interface {
	SaveTemp(src io.Reader, originalName string) (tempPath string, size int64, hash string, err error)
	Finalize(tempPath, hash string) (finalPath string, err error)
	Delete(path string) error
}

type LocalStorage struct {
	BaseDir string
}

func NewLocalStorage(baseDir string) *LocalStorage {
	return &LocalStorage{BaseDir: baseDir}
}

func (s *LocalStorage) SaveTemp(src io.Reader, originalName string) (string, int64, string, error) {
	tmpDir := filepath.Join(s.BaseDir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return "", 0, "", err
	}

	tmpFile, err := os.CreateTemp(tmpDir, "upload-*")
	if err != nil {
		return "", 0, "", err
	}
	defer tmpFile.Close()

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	size, err := io.Copy(writer, src)
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", 0, "", err
	}

	sum := hex.EncodeToString(hasher.Sum(nil))
	return tmpFile.Name(), size, sum, nil
}

func (s *LocalStorage) Finalize(tempPath, hash string) (string, error) {
	objectsDir := filepath.Join(s.BaseDir, "objects")
	if err := os.MkdirAll(objectsDir, 0o755); err != nil {
		return "", err
	}

	finalPath := filepath.Join(objectsDir, fmt.Sprintf("%s.bin", hash))

	_, err := os.Stat(finalPath)
	if err == nil {
		_ = os.Remove(tempPath)
		return finalPath, nil
	}

	if err := os.Rename(tempPath, finalPath); err != nil {
		return "", err
	}

	return finalPath, nil
}

func (s *LocalStorage) Delete(path string) error {
	return os.Remove(path)
}
