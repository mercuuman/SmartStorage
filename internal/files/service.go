package files

import (
	"bytes"
	"context"
	"diplom/internal/compression"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type Service struct {
	repo        *Repository
	storage     Storage
	compression *compression.Manager
}

func NewService(
	repo *Repository,
	storage Storage,
	compressionManager *compression.Manager,
) *Service {
	return &Service{
		repo:        repo,
		storage:     storage,
		compression: compressionManager,
	}
}

func ptrString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (s *Service) Upload(
	ctx context.Context,
	userID string,
	filename string,
	file io.Reader,
) (*File, error) {

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	hash := CalculateSHA256(data)

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback(ctx)

	existingPhysical, err := s.repo.FindPhysicalByHash(
		ctx,
		tx,
		hash,
	)

	var physicalID string

	// DUPLICATE
	if err == nil {

		err = s.repo.IncrementPhysicalReference(
			ctx,
			tx,
			existingPhysical.ID,
		)

		if err != nil {
			return nil, err
		}

		physicalID = existingPhysical.ID

	} else {

		compressor, compressedData, err := s.compression.SelectBest(data)
		if err != nil {
			return nil, err
		}

		originalSize := int64(len(data))
		compressedSize := int64(len(compressedData))

		var (
			path              string
			algorithm         *string
			compressedSizePtr *int64
			ratio             *float64
		)

		// 🔍 Проверяем: если сжатие не выгодно — сохраняем оригинал
		if compressedSize >= originalSize {
			// Сохраняем исходные данные без сжатия
			path, _, err = s.storage.SaveCompressed( // ← _ вместо finalSize
				bytes.NewReader(data),
				filepath.Ext(filename),
			)
			// Поля сжатия остаются nil
		} else {
			// Сжатие выгодно — сохраняем сжатые данные
			path, _, err = s.storage.SaveCompressed( // ← _ вместо finalSize
				bytes.NewReader(compressedData),
				filepath.Ext(filename),
			)
			if err != nil {
				return nil, err
			}

			alg := compressor.Name()
			algorithm = &alg
			compressedSizePtr = &compressedSize
			r := float64(compressedSize) / float64(originalSize)
			ratio = &r
		}

		if err != nil {
			return nil, err
		}

		physical := &PhysicalFile{
			HashSHA256:           hash,
			StoragePath:          path,
			OriginalSize:         originalSize,
			CompressedSize:       compressedSizePtr, // nil если не сжимали
			CompressionAlgorithm: algorithm,         // nil если не сжимали
			CompressionRatio:     ratio,             // nil если не сжимали
			ReferenceCount:       1,
		}
		err = s.repo.CreatePhysical(
			ctx,
			tx,
			physical,
		)

		if err != nil {
			return nil, err
		}

		physicalID = physical.ID
	}

	existingFile, err := s.repo.FindByUserAndFilename(
		ctx,
		tx,
		userID,
		filename,
	)

	var logicalFile *File
	var versionNumber int

	if err == nil {

		// FILE EXISTS -> NEW VERSION

		logicalFile = existingFile

		latestVersion, err :=
			s.repo.GetLatestVersionNumber(
				ctx,
				tx,
				logicalFile.ID,
			)

		if err != nil {
			return nil, err
		}

		versionNumber = latestVersion + 1

	} else {

		// NEW FILE

		logicalFile = &File{
			UserID:    userID,
			Filename:  filename,
			IsDeleted: false,
		}

		err = s.repo.CreateFile(
			ctx,
			tx,
			logicalFile,
		)

		if err != nil {
			return nil, err
		}

		versionNumber = 1
	}

	if err != nil {
		return nil, err
	}

	version := &FileVersion{
		FileID:         logicalFile.ID,
		PhysicalFileID: physicalID,
		VersionNumber:  versionNumber,
		UploadedBy:     userID,
	}

	err = s.repo.CreateFileVersion(
		ctx,
		tx,
		version,
	)

	if err != nil {
		return nil, err
	}

	err = s.repo.SetCurrentVersion(
		ctx,
		tx,
		logicalFile.ID,
		version.ID,
	)

	if err != nil {
		return nil, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	logicalFile.CurrentVersionID = &version.ID

	return logicalFile, nil
}

func (s *Service) List(ctx context.Context, userID string) ([]FileListItem, error) {
	return s.repo.GetAllByUserID(ctx, userID)
}

func (s *Service) GetDownloadFile(ctx context.Context, id string) (*DownloadFile, error) {
	return s.repo.GetByIDWithPhysical(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	physical, err := s.repo.GetCurrentPhysicalByFileID(ctx, tx, id)
	if err != nil {
		return err
	}

	if err := s.repo.MarkDeleted(ctx, tx, id); err != nil {
		return err
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
			return err
		}
	}

	return tx.Commit(ctx)
}
func (s *Service) Download(
	ctx context.Context,
	fileID string,
) (io.ReadCloser, string, error) {

	file, err := s.repo.GetByIDWithPhysical(
		ctx,
		fileID,
	)

	if err != nil {
		return nil, "", err
	}

	src, err := os.Open(file.StoragePath)
	if err != nil {
		return nil, "", err
	}

	// файл не сжат
	if file.CompressionAlgorithm == nil {
		return src, file.Filename, nil
	}

	compressor :=
		s.compression.GetByName(
			*file.CompressionAlgorithm,
		)

	if compressor == nil {
		src.Close()
		return nil, "", errors.New("compressor not found")
	}

	pr, pw := io.Pipe()

	go func() {

		defer pw.Close()
		defer src.Close()

		err := compressor.Decompress(
			src,
			pw,
		)

		if err != nil {
			pw.CloseWithError(err)
		}
	}()

	return pr, file.Filename, nil
}

func (s *Service) GetVersionHistory(
	ctx context.Context,
	fileID string,
) ([]FileVersionInfo, error) {

	return s.repo.GetVersionsByFileID(
		ctx,
		fileID,
	)
}
func (s *Service) RestoreVersion(
	ctx context.Context,
	fileID string,
	versionNumber int,
) error {

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	version, err := s.repo.GetVersionByNumber(
		ctx,
		tx,
		fileID,
		versionNumber,
	)

	if err != nil {
		return err
	}

	err = s.repo.SetCurrentVersion(
		ctx,
		tx,
		fileID,
		version.ID,
	)

	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Service: GetFileDetails — получает полную информацию о файле
func (s *Service) GetFileDetails(ctx context.Context, fileID string) (*FileDetails, error) {
	details, err := s.repo.GetFileWithDetails(ctx, fileID)
	if err != nil {
		return nil, err
	}

	// Опционально: скрываем чувствительные данные
	// details.PhysicalFile.StoragePath = "" // если не хотите отдавать путь

	return details, nil
}
