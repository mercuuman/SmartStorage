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

func (r *Repository) GetByIDWithPhysical(
	ctx context.Context,
	id string,
) (*DownloadFile, error) {

	query := `
		SELECT
			f.id,f.filename,pf.storage_path,pf.compression_algorithm
		FROM files f
		JOIN file_versions fv
			ON fv.id = f.current_version_id
		JOIN physical_files pf
			ON pf.id = fv.physical_file_id
		WHERE f.id = $1
		  AND f.is_deleted = false
	`

	var file DownloadFile

	var compressionAlg sql.NullString

	err := r.db.QueryRow(
		ctx, query, id,
	).Scan(
		&file.ID, &file.Filename, &file.StoragePath, &compressionAlg,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if compressionAlg.Valid {
		v := compressionAlg.String
		file.CompressionAlgorithm = &v
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
func (r *Repository) FindByUserAndFilename(
	ctx context.Context,
	tx pgx.Tx,
	userID string,
	filename string,
) (*File, error) {

	query := `
		SELECT
			id,
			user_id,
			organization_id,
			filename,0
			current_version_id,
			is_deleted,
			created_at,
			updated_at
		FROM files
		WHERE user_id = $1
		  AND filename = $2
		  AND is_deleted = false
		LIMIT 1
	`

	var file File

	err := tx.QueryRow(
		ctx,
		query,
		userID,
		filename,
	).Scan(
		&file.ID,
		&file.UserID,
		&file.OrganizationID,
		&file.Filename,
		&file.CurrentVersionID,
		&file.IsDeleted,
		&file.CreatedAt,
		&file.UpdatedAt,
	)

	if err != nil {

		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &file, nil
}

func (r *Repository) GetLatestVersionNumber(
	ctx context.Context,
	tx pgx.Tx,
	fileID string,
) (int, error) {

	var version int

	err := tx.QueryRow(
		ctx,
		`
		SELECT COALESCE(MAX(version_number), 0)
		FROM file_versions
		WHERE file_id = $1
		`,
		fileID,
	).Scan(&version)

	return version, err
}

func (r *Repository) GetVersionsByFileID(
	ctx context.Context,
	fileID string,
) ([]FileVersionInfo, error) {

	query := `
		SELECT
			fv.id,fv.version_number,fv.created_at,
			pf.id,pf.original_size,pf.compressed_size,pf.compression_algorithm,pf.compression_ratio

		FROM file_versions fv

		JOIN physical_files pf
			ON pf.id = fv.physical_file_id

		WHERE fv.file_id = $1

		ORDER BY fv.version_number DESC
	`

	rows, err := r.db.Query(
		ctx,
		query,
		fileID,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var versions []FileVersionInfo

	for rows.Next() {

		var v FileVersionInfo

		var compressedSize sql.NullInt64
		var algorithm sql.NullString
		var ratio sql.NullFloat64

		err := rows.Scan(
			&v.ID, &v.VersionNumber, &v.CreatedAt,
			&v.PhysicalFileID, &v.OriginalSize, &compressedSize, &algorithm, &ratio,
		)

		if err != nil {
			return nil, err
		}

		if compressedSize.Valid {
			val := compressedSize.Int64
			v.CompressedSize = &val
		}

		if algorithm.Valid {
			val := algorithm.String
			v.CompressionAlgorithm = &val
		}

		if ratio.Valid {
			val := ratio.Float64
			v.CompressionRatio = &val
		}

		versions = append(versions, v)
	}

	return versions, rows.Err()
}

func (r *Repository) GetVersionByNumber(
	ctx context.Context,
	tx pgx.Tx,
	fileID string,
	versionNumber int,
) (*FileVersion, error) {

	query := `
		SELECT
			id,file_id,physical_file_id,version_number,uploaded_by,created_at
		FROM file_versions
		WHERE file_id = $1
		  AND version_number = $2
	`

	var v FileVersion

	err := tx.QueryRow(
		ctx, query, fileID, versionNumber,
	).Scan(
		&v.ID, &v.FileID, &v.PhysicalFileID, &v.VersionNumber, &v.UploadedBy, &v.CreatedAt,
	)

	if err != nil {

		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &v, nil
}

// Repository: GetFileWithDetails - получает файл со всей связанной информацией
func (r *Repository) GetFileWithDetails(
	ctx context.Context,
	fileID string,
) (*FileDetails, error) {
	query := `
		SELECT
			-- Logical file fields
			f.id,
			f.user_id,
			f.organization_id,
			f.filename,
			f.current_version_id,
			f.is_deleted,
			f.created_at,
			f.updated_at,
			-- Current version fields
			fv.id as version_id,
			fv.version_number,
			fv.uploaded_by,
			fv.created_at as version_created_at,
			-- Physical file fields
			pf.id as physical_id,
			pf.hash_sha256,
			pf.storage_path,
			pf.original_size,
			pf.compressed_size,
			pf.compression_algorithm,
			pf.compression_ratio,
			pf.reference_count,
			pf.created_at as physical_created_at
		FROM files f
		LEFT JOIN file_versions fv 
			ON fv.id = f.current_version_id
		LEFT JOIN physical_files pf 
			ON pf.id = fv.physical_file_id
		WHERE f.id = $1
		  AND f.is_deleted = false
	`

	var details FileDetails
	var orgID, currentVersionID, versionID, uploadedBy, physicalID sql.NullString
	var compressedSize sql.NullInt64
	var compressionAlg sql.NullString
	var compressionRatio sql.NullFloat64
	var versionCreatedAt, physicalCreatedAt sql.NullTime

	err := r.db.QueryRow(ctx, query, fileID).Scan(
		&details.File.ID,
		&details.File.UserID,
		&orgID,
		&details.File.Filename,
		&currentVersionID,
		&details.File.IsDeleted,
		&details.File.CreatedAt,
		&details.File.UpdatedAt,
		&versionID,
		&details.CurrentVersion.Number,
		&uploadedBy,
		&versionCreatedAt,
		&physicalID,
		&details.PhysicalFile.HashSHA256,
		&details.PhysicalFile.StoragePath,
		&details.PhysicalFile.OriginalSize,
		&compressedSize,
		&compressionAlg,
		&compressionRatio,
		&details.PhysicalFile.ReferenceCount,
		&physicalCreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Заполняем опциональные поля логического файла
	if orgID.Valid {
		v := orgID.String
		details.File.OrganizationID = &v
	}
	if currentVersionID.Valid {
		v := currentVersionID.String
		details.File.CurrentVersionID = &v
	}

	// Заполняем данные версии (если есть)
	if versionID.Valid {
		details.CurrentVersion.ID = versionID.String
		details.CurrentVersion.HasData = true
		if uploadedBy.Valid {
			v := uploadedBy.String
			details.CurrentVersion.UploadedBy = v
		}
		if versionCreatedAt.Valid {
			details.CurrentVersion.CreatedAt = versionCreatedAt.Time
		}
	}

	// Заполняем данные физического файла (если есть)
	if physicalID.Valid {
		details.PhysicalFile.ID = physicalID.String
		details.PhysicalFile.HasData = true

		if compressedSize.Valid {
			v := compressedSize.Int64
			details.PhysicalFile.CompressedSize = &v
		}
		if compressionAlg.Valid {
			v := compressionAlg.String
			details.PhysicalFile.CompressionAlgorithm = &v
		}
		if compressionRatio.Valid {
			v := compressionRatio.Float64
			details.PhysicalFile.CompressionRatio = &v
		}
		if physicalCreatedAt.Valid {
			details.PhysicalFile.CreatedAt = physicalCreatedAt.Time
		}
	}

	return &details, nil
}
