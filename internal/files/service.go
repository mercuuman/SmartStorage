package files

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"diplom/internal/compression"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"log"
	"os"
	"path/filepath"
)

type Service struct {
	repo        *Repository
	storage     Storage
	compression *compression.Manager
}

func NewService(repo *Repository, storage Storage, cm *compression.Manager) *Service {
	return &Service{repo: repo, storage: storage, compression: cm}
}

func ptrString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// ==================== FILE UPLOAD & MANAGEMENT ====================

func (s *Service) Upload(ctx context.Context, userID, filename string, file io.Reader, folderID *string) (*File, error) {
	// 🔥 Создаём временный файл для стриминга (не грузим всё в RAM)
	tmpFile, err := os.CreateTemp("", "upload-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath)
	}()

	hasher := sha256.New()
	teeReader := io.TeeReader(file, hasher)

	written, err := io.Copy(tmpFile, teeReader)
	if err != nil {
		log.Printf("❌ [Upload] write to temp: %v", err)
		return nil, fmt.Errorf("failed to write to temp file: %w", err)
	}

	if _, err := tmpFile.Seek(0, 0); err != nil {
		log.Printf("❌ [Upload] seek temp: %v", err)
		return nil, fmt.Errorf("failed to seek temp file: %w", err)
	}

	hash := fmt.Sprintf("%x", hasher.Sum(nil))
	originalSize := written

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		log.Printf("❌ [Upload] begin tx: %v", err)
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Проверяем дедупликацию
	existingPhysical, err := s.repo.FindPhysicalByHash(ctx, tx, hash)
	var physicalID string

	if err == nil {
		// дедупликация
		if err := s.repo.IncrementPhysicalReference(ctx, tx, existingPhysical.ID); err != nil {
			log.Printf("❌ [Upload] increment ref: %v", err)
			return nil, err
		}
		physicalID = existingPhysical.ID
	} else {
		// Новый файл — сжимаем
		path, algorithm, compressedSize, ratio, err := s.compressAndSave(tmpFile, filename, originalSize)
		if err != nil {
			log.Printf("❌ [Upload] compressAndSave failed: %v", err)
			return nil, err
		}
		var compressedSizePtr *int64
		if compressedSize > 0 {
			compressedSizePtr = &compressedSize
		}

		physical := &PhysicalFile{
			HashSHA256:           hash,
			StoragePath:          path,
			OriginalSize:         originalSize,
			CompressedSize:       compressedSizePtr,
			CompressionAlgorithm: algorithm,
			CompressionRatio:     ratio,
			ReferenceCount:       1,
		}
		if err := s.repo.CreatePhysical(ctx, tx, physical); err != nil {
			log.Printf("❌ [Upload] create physical: %v", err)
			return nil, err
		}
		physicalID = physical.ID
	}

	// Работа с логическим файлом
	existingFile, err := s.repo.FindByUserAndFilename(ctx, tx, userID, filename)
	var logicalFile *File
	var versionNumber int

	if err == nil {
		// Файл существует — новая версия
		logicalFile = existingFile
		latest, err := s.repo.GetLatestVersionNumber(ctx, tx, logicalFile.ID)
		if err != nil {
			return nil, err
		}
		versionNumber = latest + 1
	} else {
		// Новый логический файл
		logicalFile = &File{
			UserID:    userID,
			Filename:  filename,
			FolderID:  folderID,
			IsDeleted: false,
		}
		if err := s.repo.CreateFile(ctx, tx, logicalFile); err != nil {
			return nil, err
		}
		versionNumber = 1
	}

	version := &FileVersion{
		FileID:         logicalFile.ID,
		PhysicalFileID: physicalID,
		VersionNumber:  versionNumber,
		UploadedBy:     userID,
	}
	if err := s.repo.CreateFileVersion(ctx, tx, version); err != nil {
		return nil, err
	}
	if err := s.repo.SetCurrentVersion(ctx, tx, logicalFile.ID, version.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	logicalFile.CurrentVersionID = &version.ID
	return logicalFile, nil
}

func (s *Service) compressAndSave(srcFile *os.File, filename string, originalSize int64) (string, *string, int64, *float64, error) {
	// Перематываем в начало
	if _, err := srcFile.Seek(0, 0); err != nil {
		return "", nil, 0, nil, err
	}

	// Создаём временный файл для сжатия
	tmpCompressed, err := os.CreateTemp("", "compressed-*")
	if err != nil {
		return "", nil, 0, nil, err
	}
	tmpCompressedPath := tmpCompressed.Name()
	defer func() {
		tmpCompressed.Close()
		os.Remove(tmpCompressedPath)
	}()

	// Пытаемся сжать
	compressor, err := s.compression.SelectBestForStream(srcFile, tmpCompressed)
	if err != nil {
		return "", nil, 0, nil, err
	}

	// Получаем размер сжатого файла
	tmpCompressed.Sync()
	stat, err := tmpCompressed.Stat()
	if err != nil {
		return "", nil, 0, nil, err
	}
	compressedSize := stat.Size()

	var path string
	var algorithm *string
	var ratio *float64

	// Перематываем сжатый файл для чтения
	if _, err := tmpCompressed.Seek(0, 0); err != nil {
		return "", nil, 0, nil, err
	}

	if compressedSize >= originalSize {
		// Сжатие невыгодно — сохраняем оригинал
		if _, err := srcFile.Seek(0, 0); err != nil {
			return "", nil, 0, nil, err
		}
		path, _, err = s.storage.SaveCompressed(srcFile, filepath.Ext(filename))
		if err != nil {
			return "", nil, 0, nil, err
		}
	} else {
		// Сжатие выгодно — сохраняем сжатое
		path, _, err = s.storage.SaveCompressed(tmpCompressed, filepath.Ext(filename))
		if err != nil {
			return "", nil, 0, nil, err
		}
		alg := compressor.Name()
		algorithm = &alg
		r := float64(compressedSize) / float64(originalSize)
		ratio = &r
	}

	return path, algorithm, compressedSize, ratio, nil
}

func (s *Service) List(ctx context.Context, userID string, folderID *string) ([]FileListItem, error) {
	return s.repo.GetAllByUserID(ctx, userID, folderID)
}

func (s *Service) GetDownloadFile(ctx context.Context, id string) (*DownloadFile, error) {
	return s.repo.GetByIDWithPhysical(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var userID string
	var folderID sql.NullString
	err = tx.QueryRow(ctx, `SELECT user_id, folder_id FROM files WHERE id = $1`, id).Scan(&userID, &folderID)
	if err != nil {
		return err
	}

	trashID, err := s.repo.GetOrCreateTrashFolder(ctx, tx, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE files SET folder_id = $1 WHERE id = $2`, trashID, id)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) Download(ctx context.Context, fileID string) (io.ReadCloser, string, error) {
	file, err := s.repo.GetByIDWithPhysical(ctx, fileID)
	if err != nil {
		return nil, "", err
	}
	src, err := os.Open(file.StoragePath)
	if err != nil {
		return nil, "", err
	}
	if file.CompressionAlgorithm == nil {
		return src, file.Filename, nil
	}
	compressor := s.compression.GetByName(*file.CompressionAlgorithm)
	if compressor == nil {
		src.Close()
		return nil, "", errors.New("compressor not found")
	}
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		defer src.Close()
		if err := compressor.Decompress(src, pw); err != nil {
			pw.CloseWithError(err)
		}
	}()
	return pr, file.Filename, nil
}

func (s *Service) GetVersionHistory(ctx context.Context, fileID string) ([]FileVersionInfo, error) {
	return s.repo.GetVersionsByFileID(ctx, fileID)
}

func (s *Service) RestoreVersion(ctx context.Context, fileID string, versionNumber int) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	version, err := s.repo.GetVersionByNumber(ctx, tx, fileID, versionNumber)
	if err != nil {
		return err
	}
	if err := s.repo.SetCurrentVersion(ctx, tx, fileID, version.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) GetFileDetails(ctx context.Context, fileID string) (*FileDetails, error) {
	return s.repo.GetFileWithDetails(ctx, fileID)
}

// ==================== FOLDER OPERATIONS ====================

func (s *Service) CreateFolder(ctx context.Context, userID, name string, parentID *string) (*Folder, error) {
	if name == "" {
		return nil, errors.New("folder name cannot be empty")
	}
	if parentID != nil {
		exists, err := s.repo.FolderExists(ctx, nil, userID, *parentID, name)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("folder with this name already exists")
		}
	}
	folder := &Folder{ID: uuid.NewString(), UserID: userID, ParentID: parentID, Name: name, IsSystem: false}
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := s.repo.CreateFolder(ctx, tx, folder); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return folder, nil
}

func (s *Service) GetFolders(ctx context.Context, userID string, parentID *string) ([]Folder, error) {
	return s.repo.GetFoldersByParent(ctx, nil, userID, parentID)
}

func (s *Service) GetFolderContents(ctx context.Context, userID, folderID string) (*FolderContents, error) {
	folder, err := s.repo.GetFolderByID(ctx, nil, folderID)
	if err != nil {
		return nil, ErrNotFound
	}
	if folder.UserID != userID {
		return nil, errors.New("access denied")
	}
	f, subfolders, files, err := s.repo.GetFolderWithChildren(ctx, nil, folderID)
	if err != nil {
		return nil, err
	}
	return &FolderContents{Folder: *f, Subfolders: subfolders, Files: files}, nil
}

func (s *Service) DeleteFolder(ctx context.Context, userID, folderID string) error {
	folder, err := s.repo.GetFolderByID(ctx, nil, folderID)
	if err != nil {
		return ErrNotFound
	}
	if folder.UserID != userID {
		return errors.New("access denied")
	}
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.repo.DeleteFolder(ctx, tx, folderID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ==================== TRASH OPERATIONS ====================

func (s *Service) EmptyTrash(ctx context.Context, userID string) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	trashID, err := s.repo.GetOrCreateTrashFolder(ctx, tx, userID)
	if err != nil {
		return err
	}
	files, err := s.repo.GetTrashFiles(ctx, tx, userID)
	if err != nil {
		return err
	}
	for _, f := range files {
		physical, err := s.repo.GetCurrentPhysicalByFileID(ctx, tx, f.ID)
		if err != nil {
			continue
		}
		count, err := s.repo.DecrementPhysicalReference(ctx, tx, physical.ID)
		if err != nil {
			return err
		}
		if count <= 0 {
			if err := s.repo.DeletePhysical(ctx, tx, physical.ID); err != nil {
				return err
			}
			if err := s.storage.Delete(physical.StoragePath); err != nil {
				log.Printf("⚠️ failed to delete physical file %s: %v", physical.StoragePath, err)
			}
		}
	}
	_, err = tx.Exec(ctx, `DELETE FROM files WHERE folder_id = $1 AND user_id = $2`, trashID, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM folders WHERE parent_id = $1 AND user_id = $2`, trashID, userID)
	return tx.Commit(ctx)
}

func (s *Service) RestoreFromTrash(ctx context.Context, userID string, fileID, folderID, targetParentID *string) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	trashID, _ := s.repo.GetOrCreateTrashFolder(ctx, tx, userID)
	if fileID != nil {
		_, err = tx.Exec(ctx, `UPDATE files SET folder_id = $1 WHERE id = $2 AND user_id = $3 AND folder_id = $4`, targetParentID, *fileID, userID, trashID)
	} else if folderID != nil {
		_, err = tx.Exec(ctx, `UPDATE folders SET parent_id = $1 WHERE id = $2 AND user_id = $3 AND parent_id = $4`, targetParentID, *folderID, userID, trashID)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MoveFile перемещает файл в указанную папку (или в корень, если folderID = nil)
func (s *Service) MoveFile(ctx context.Context, userID, fileID string, folderID *string) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Проверяем, что файл принадлежит пользователю
	file, err := s.repo.GetFileByID(ctx, tx, fileID)
	if err != nil {
		return err
	}
	if file.UserID != userID {
		return errors.New("access denied")
	}

	// Если целевая папка указана — проверяем, что она принадлежит пользователю
	if folderID != nil {
		folder, err := s.repo.GetFolderByID(ctx, tx, *folderID)
		if err != nil {
			return err
		}
		if folder.UserID != userID {
			return errors.New("access denied")
		}
		if folder.IsSystem {
			return errors.New("cannot move to system folder")
		}
	}

	if err := s.repo.MoveFileToFolder(ctx, tx, fileID, folderID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MoveFolder перемещает папку в другую папку
func (s *Service) MoveFolder(ctx context.Context, userID, folderID string, parentID *string) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	folder, err := s.repo.GetFolderByID(ctx, tx, folderID)
	if err != nil {
		return err
	}
	if folder.UserID != userID {
		return errors.New("access denied")
	}
	if folder.IsSystem {
		return errors.New("cannot move system folder")
	}

	// Нельзя переместить папку саму в себя
	if parentID != nil && *parentID == folderID {
		return errors.New("cannot move folder into itself")
	}

	// Проверка на циклическую ссылку: нельзя переместить папку в её же потомка
	if parentID != nil {
		if isDescendant, err := s.repo.IsDescendantOf(ctx, tx, *parentID, folderID); err != nil {
			return err
		} else if isDescendant {
			return errors.New("cannot move folder into its descendant")
		}
	}

	// Если целевая папка указана — проверяем права
	if parentID != nil {
		target, err := s.repo.GetFolderByID(ctx, tx, *parentID)
		if err != nil {
			return err
		}
		if target.UserID != userID {
			return errors.New("access denied")
		}
		if target.IsSystem {
			return errors.New("cannot move to system folder")
		}
	}

	if err := s.repo.MoveFolderToParent(ctx, tx, folderID, parentID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
