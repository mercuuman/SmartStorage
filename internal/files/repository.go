package files

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.db.Begin(ctx)
}

func (r *Repository) FindPhysicalByHash(ctx context.Context, tx pgx.Tx, hash string) (*PhysicalFile, error) {
	query := `
		SELECT id, hash_sha256, storage_path, original_size, compressed_size,
		       compression_algorithm, compression_ratio, reference_count, created_at
		FROM physical_files
		WHERE hash_sha256 = $1
	`

	var pf PhysicalFile
	var compressedSize sql.NullInt64
	var compressionAlg sql.NullString
	var compressionRatio sql.NullFloat64

	err := tx.QueryRow(ctx, query, hash).Scan(
		&pf.ID,
		&pf.HashSHA256,
		&pf.StoragePath,
		&pf.OriginalSize,
		&compressedSize,
		&compressionAlg,
		&compressionRatio,
		&pf.ReferenceCount,
		&pf.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if compressedSize.Valid {
		v := compressedSize.Int64
		pf.CompressedSize = &v
	}
	if compressionAlg.Valid {
		v := compressionAlg.String
		pf.CompressionAlgorithm = &v
	}
	if compressionRatio.Valid {
		v := compressionRatio.Float64
		pf.CompressionRatio = &v
	}

	return &pf, nil
}

func (r *Repository) CreatePhysical(ctx context.Context, tx pgx.Tx, pf *PhysicalFile) error {
	query := `
		INSERT INTO physical_files (
			hash_sha256,storage_path,original_size,compressed_size,
			compression_algorithm,compression_ratio,reference_count
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`

	var compressedSize any
	if pf.CompressedSize != nil {
		compressedSize = *pf.CompressedSize
	} else {
		compressedSize = nil
	}

	var compressionAlg any
	if pf.CompressionAlgorithm != nil {
		compressionAlg = *pf.CompressionAlgorithm
	} else {
		compressionAlg = nil
	}

	var compressionRatio any
	if pf.CompressionRatio != nil {
		compressionRatio = *pf.CompressionRatio
	} else {
		compressionRatio = nil
	}

	return tx.QueryRow(
		ctx,
		query,
		pf.HashSHA256,
		pf.StoragePath,
		pf.OriginalSize,
		compressedSize,
		compressionAlg,
		compressionRatio,
		pf.ReferenceCount,
	).Scan(&pf.ID, &pf.CreatedAt)
}

func (r *Repository) IncrementPhysicalReference(ctx context.Context, tx pgx.Tx, physicalID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE physical_files
		SET reference_count = reference_count + 1
		WHERE id = $1
	`, physicalID)
	return err
}

func (r *Repository) DecrementPhysicalReference(ctx context.Context, tx pgx.Tx, physicalID string) (int, error) {
	_, err := tx.Exec(ctx, `
		UPDATE physical_files
		SET reference_count = reference_count - 1
		WHERE id = $1
	`, physicalID)
	if err != nil {
		return 0, err
	}

	var count int
	err = tx.QueryRow(ctx, `
		SELECT reference_count
		FROM physical_files
		WHERE id = $1
	`, physicalID).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (r *Repository) DeletePhysical(ctx context.Context, tx pgx.Tx, physicalID string) error {
	_, err := tx.Exec(ctx, `
		DELETE FROM physical_files
		WHERE id = $1
	`, physicalID)
	return err
}

func (r *Repository) CreateFile(ctx context.Context, tx pgx.Tx, file *File) error {
	query := `
		INSERT INTO files (
			user_id,
			organization_id,
			filename,
			current_version_id,
			is_deleted
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`

	return tx.QueryRow(
		ctx,
		query,
		file.UserID,
		file.OrganizationID,
		file.Filename,
		file.CurrentVersionID,
		file.IsDeleted,
	).Scan(&file.ID, &file.CreatedAt, &file.UpdatedAt)
}

func (r *Repository) CreateFileVersion(ctx context.Context, tx pgx.Tx, version *FileVersion) error {
	query := `
		INSERT INTO file_versions (
			file_id,
			physical_file_id,
			version_number,
			uploaded_by
		)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`

	return tx.QueryRow(
		ctx,
		query,
		version.FileID,
		version.PhysicalFileID,
		version.VersionNumber,
		version.UploadedBy,
	).Scan(&version.ID, &version.CreatedAt)
}

func (r *Repository) SetCurrentVersion(ctx context.Context, tx pgx.Tx, fileID, versionID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE files
		SET current_version_id = $1, updated_at = NOW()
		WHERE id = $2
	`, versionID, fileID)
	return err
}

func (r *Repository) GetAllByUserID(ctx context.Context, userID string) ([]FileListItem, error) {
	query := `
		SELECT id, user_id, organization_id, filename, current_version_id, is_deleted, created_at, updated_at
		FROM files
		WHERE user_id = $1 AND is_deleted = false
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileListItem
	for rows.Next() {
		var item FileListItem
		var orgID sql.NullString
		var currentVersionID sql.NullString

		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&orgID,
			&item.Filename,
			&currentVersionID,
			&item.IsDeleted,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if orgID.Valid {
			v := orgID.String
			item.OrganizationID = &v
		}
		if currentVersionID.Valid {
			v := currentVersionID.String
			item.CurrentVersionID = &v
		}

		files = append(files, item)
	}

	return files, rows.Err()
}

func (r *Repository) GetByIDWithPhysical(ctx context.Context, id string) (*DownloadFile, error) {
	query := `
		SELECT f.id, f.filename, pf.storage_path
		FROM files f
		JOIN file_versions fv ON fv.id = f.current_version_id
		JOIN physical_files pf ON pf.id = fv.physical_file_id
		WHERE f.id = $1 AND f.is_deleted = false
	`

	var file DownloadFile
	err := r.db.QueryRow(ctx, query, id).Scan(
		&file.ID,
		&file.Filename,
		&file.StoragePath,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &file, nil
}

func (r *Repository) MarkDeleted(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := tx.Exec(ctx, `
		UPDATE files
		SET is_deleted = true, updated_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

func (r *Repository) GetCurrentPhysicalByFileID(ctx context.Context, tx pgx.Tx, fileID string) (*PhysicalFile, error) {
	query := `
		SELECT pf.id, pf.hash_sha256, pf.storage_path, pf.original_size,
		       pf.compressed_size, pf.compression_algorithm, pf.compression_ratio,
		       pf.reference_count, pf.created_at
		FROM files f
		JOIN file_versions fv ON fv.id = f.current_version_id
		JOIN physical_files pf ON pf.id = fv.physical_file_id
		WHERE f.id = $1
	`

	var pf PhysicalFile
	var compressedSize sql.NullInt64
	var compressionAlg sql.NullString
	var compressionRatio sql.NullFloat64

	err := tx.QueryRow(ctx, query, fileID).Scan(
		&pf.ID,
		&pf.HashSHA256,
		&pf.StoragePath,
		&pf.OriginalSize,
		&compressedSize,
		&compressionAlg,
		&compressionRatio,
		&pf.ReferenceCount,
		&pf.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if compressedSize.Valid {
		v := compressedSize.Int64
		pf.CompressedSize = &v
	}
	if compressionAlg.Valid {
		v := compressionAlg.String
		pf.CompressionAlgorithm = &v
	}
	if compressionRatio.Valid {
		v := compressionRatio.Float64
		pf.CompressionRatio = &v
	}

	return &pf, nil
}

// Проверка, используется ли физический файл другими версиями
func (r *Repository) IsPhysicalFileUsed(ctx context.Context, tx pgx.Tx, physicalID string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
        SELECT EXISTS(
            SELECT 1 FROM file_versions 
            WHERE physical_file_id = $1
        )
    `, physicalID).Scan(&exists)
	return exists, err
}
func (r *Repository) GetPhysicalByHash(
	hash string,
) (*PhysicalFile, error) {

	query := `
		SELECT
id,hash_sha256,storage_path,original_size,compressed_size,compression_algorithm,compression_ratio,reference_count,created_at
		FROM physical_files
		WHERE hash_sha256 = $1
	`

	var file PhysicalFile

	err := r.db.QueryRow(
		context.Background(),
		query,
		hash,
	).Scan(&file.ID, &file.HashSHA256, &file.StoragePath, &file.OriginalSize, &file.CompressedSize,
		&file.CompressionAlgorithm, &file.CompressionRatio, &file.ReferenceCount, &file.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &file, nil
}
