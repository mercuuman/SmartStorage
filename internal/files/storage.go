package files

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/google/uuid"
	"io"
	"os"
	"path/filepath"
)

type Storage interface {
	SaveTemp(src io.Reader, originalName string) (tempPath string, size int64, hash string, err error)
	Finalize(tempPath, hash string) (finalPath string, err error)
	Delete(path string) error
	SaveCompressed(reader io.Reader, ext string) (string, int64, error)
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

// ✅ Правильный receiver: *LocalStorage
func (s *LocalStorage) SaveCompressed(
	reader io.Reader,
	ext string,
) (string, int64, error) {

	// ✅ Создаём директорию, если не существует
	if err := os.MkdirAll(s.BaseDir, 0o755); err != nil { // ✅ было: s.basePath
		return "", 0, err
	}

	filename := uuid.New().String() + ext
	fullPath := filepath.Join(s.BaseDir, filename) // ✅ было: s.basePath

	dst, err := os.Create(fullPath)
	if err != nil {
		return "", 0, err
	}
	defer dst.Close()

	size, err := io.Copy(dst, reader)
	if err != nil {
		return "", 0, err
	}

	return fullPath, size, nil
}
