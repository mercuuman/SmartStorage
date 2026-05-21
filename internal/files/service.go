package files

import (
	"context"
	"io"
)

type Service struct {
	repo    *Repository
	storage Storage
}

func NewService(repo *Repository, storage Storage) *Service {
	return &Service{
		repo:    repo,
		storage: storage,
	}
}

func ptrString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (s *Service) Upload(ctx context.Context, userID, organizationID, filename string, src io.Reader) (*File, error) {
	tempPath, originalSize, hash, err := s.storage.SaveTemp(src, filename)
	if err != nil {
		return nil, err
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		_ = s.storage.Delete(tempPath)
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var physical *PhysicalFile
	physical, err = s.repo.FindPhysicalByHash(ctx, tx, hash)

	newPhysical := false
	if err != nil && err != ErrNotFound {
		_ = s.storage.Delete(tempPath)
		return nil, err
	}

	if physical == nil {
		finalPath, err := s.storage.Finalize(tempPath, hash)
		if err != nil {
			_ = s.storage.Delete(tempPath)
			return nil, err
		}

		physical = &PhysicalFile{
			HashSHA256:     hash,
			StoragePath:    finalPath,
			OriginalSize:   originalSize,
			ReferenceCount: 1,
		}

		if err := s.repo.CreatePhysical(ctx, tx, physical); err != nil {
			_ = s.storage.Delete(finalPath)
			return nil, err
		}
		newPhysical = true
	} else {
		if err := s.repo.IncrementPhysicalReference(ctx, tx, physical.ID); err != nil {
			_ = s.storage.Delete(tempPath)
			return nil, err
		}
		_ = s.storage.Delete(tempPath)
	}

	file := &File{
		UserID:         userID,
		OrganizationID: ptrString(organizationID),
		Filename:       filename,
		IsDeleted:      false,
	}

	if err := s.repo.CreateFile(ctx, tx, file); err != nil {
		if newPhysical {
			_ = s.storage.Delete(physical.StoragePath)
		}
		return nil, err
	}

	version := &FileVersion{
		FileID:         file.ID,
		PhysicalFileID: physical.ID,
		VersionNumber:  1,
		UploadedBy:     ptrString(userID),
	}

	if err := s.repo.CreateFileVersion(ctx, tx, version); err != nil {
		if newPhysical {
			_ = s.storage.Delete(physical.StoragePath)
		}
		return nil, err
	}

	if err := s.repo.SetCurrentVersion(ctx, tx, file.ID, version.ID); err != nil {
		if newPhysical {
			_ = s.storage.Delete(physical.StoragePath)
		}
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		if newPhysical {
			_ = s.storage.Delete(physical.StoragePath)
		}
		return nil, err
	}

	file.CurrentVersionID = &version.ID
	return file, nil
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
