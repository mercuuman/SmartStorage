package files

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// queryRow выбирает между tx и r.db для операций с одной строкой
func (r *Repository) queryRow(ctx context.Context, tx pgx.Tx, query string, args ...interface{}) pgx.Row {
	if tx != nil {
		return tx.QueryRow(ctx, query, args...)
	}
	return r.db.QueryRow(ctx, query, args...)
}

// query выбирает между tx и r.db для операций с множеством строк
func (r *Repository) query(ctx context.Context, tx pgx.Tx, query string, args ...interface{}) (pgx.Rows, error) {
	if tx != nil {
		return tx.Query(ctx, query, args...)
	}
	return r.db.Query(ctx, query, args...)
}

// exec выбирает между tx и r.db для операций изменения
func (r *Repository) exec(ctx context.Context, tx pgx.Tx, query string, args ...interface{}) (pgconn.CommandTag, error) {
	if tx != nil {
		return tx.Exec(ctx, query, args...)
	}
	return r.db.Exec(ctx, query, args...)
}

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

// ==================== PHYSICAL FILES ====================

func (r *Repository) FindPhysicalByHash(ctx context.Context, tx pgx.Tx, hash string) (*PhysicalFile, error) {
	query := `
		SELECT id, hash_sha256, storage_path, original_size, compressed_size,
		       compression_algorithm, compression_ratio, reference_count, created_at
		FROM physical_files WHERE hash_sha256 = $1`

	var pf PhysicalFile
	var compressedSize sql.NullInt64
	var compressionAlg sql.NullString
	var compressionRatio sql.NullFloat64

	err := tx.QueryRow(ctx, query, hash).Scan(
		&pf.ID, &pf.HashSHA256, &pf.StoragePath, &pf.OriginalSize,
		&compressedSize, &compressionAlg, &compressionRatio,
		&pf.ReferenceCount, &pf.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if compressedSize.Valid {
		pf.CompressedSize = &compressedSize.Int64
	}
	if compressionAlg.Valid {
		pf.CompressionAlgorithm = &compressionAlg.String
	}
	if compressionRatio.Valid {
		pf.CompressionRatio = &compressionRatio.Float64
	}
	return &pf, nil
}

func (r *Repository) CreatePhysical(ctx context.Context, tx pgx.Tx, pf *PhysicalFile) error {
	query := `
		INSERT INTO physical_files (
			hash_sha256, storage_path, original_size, compressed_size,
			compression_algorithm, compression_ratio, reference_count
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`

	return tx.QueryRow(ctx, query,
		pf.HashSHA256, pf.StoragePath, pf.OriginalSize,
		pf.CompressedSize, pf.CompressionAlgorithm, pf.CompressionRatio,
		pf.ReferenceCount,
	).Scan(&pf.ID, &pf.CreatedAt)
}

func (r *Repository) IncrementPhysicalReference(ctx context.Context, tx pgx.Tx, physicalID string) error {
	_, err := tx.Exec(ctx, `UPDATE physical_files SET reference_count = reference_count + 1 WHERE id = $1`, physicalID)
	return err
}

func (r *Repository) DecrementPhysicalReference(ctx context.Context, tx pgx.Tx, physicalID string) (int, error) {
	_, err := tx.Exec(ctx, `UPDATE physical_files SET reference_count = reference_count - 1 WHERE id = $1`, physicalID)
	if err != nil {
		return 0, err
	}
	var count int
	err = tx.QueryRow(ctx, `SELECT reference_count FROM physical_files WHERE id = $1`, physicalID).Scan(&count)
	return count, err
}

func (r *Repository) DeletePhysical(ctx context.Context, tx pgx.Tx, physicalID string) error {
	_, err := tx.Exec(ctx, `DELETE FROM physical_files WHERE id = $1`, physicalID)
	return err
}

// ==================== LOGICAL FILES ====================

func (r *Repository) CreateFile(ctx context.Context, tx pgx.Tx, file *File) error {
	query := `
		INSERT INTO files (user_id, organization_id, filename, folder_id, current_version_id, is_deleted)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`

	return tx.QueryRow(ctx, query,
		file.UserID, file.OrganizationID, file.Filename, file.FolderID,
		file.CurrentVersionID, file.IsDeleted,
	).Scan(&file.ID, &file.CreatedAt, &file.UpdatedAt)
}

func (r *Repository) CreateFileVersion(ctx context.Context, tx pgx.Tx, version *FileVersion) error {
	query := `
		INSERT INTO file_versions (file_id, physical_file_id, version_number, uploaded_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`

	return tx.QueryRow(ctx, query,
		version.FileID, version.PhysicalFileID, version.VersionNumber, version.UploadedBy,
	).Scan(&version.ID, &version.CreatedAt)
}

func (r *Repository) SetCurrentVersion(ctx context.Context, tx pgx.Tx, fileID, versionID string) error {
	_, err := tx.Exec(ctx, `UPDATE files SET current_version_id = $1, updated_at = NOW() WHERE id = $2`, versionID, fileID)
	return err
}

func (r *Repository) GetAllByUserID(ctx context.Context, userID string, folderID *string) ([]FileListItem, error) {
	query := `
		SELECT 
			f.id, f.user_id, f.organization_id, f.filename, f.folder_id, f.current_version_id, f.is_deleted, f.created_at, f.updated_at,
			pf.original_size
		FROM files f
		LEFT JOIN file_versions fv ON fv.id = f.current_version_id
		LEFT JOIN physical_files pf ON pf.id = fv.physical_file_id
		WHERE f.user_id = $1 AND f.is_deleted = false
	`
	args := []interface{}{userID}
	argIdx := 2

	if folderID != nil {
		// 🔥 Если папка указана — показываем файлы в этой папке
		query += fmt.Sprintf(" AND f.folder_id = $%d", argIdx)
		args = append(args, *folderID)
	} else {
		// 🔥 Если папка не указана — показываем только файлы в корне (folder_id IS NULL)
		query += " AND f.folder_id IS NULL"
	}
	query += " ORDER BY f.filename ASC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []FileListItem
	for rows.Next() {
		var item FileListItem
		var orgID, folderIDVal, curVerID sql.NullString
		var sizeBytes sql.NullInt64

		err := rows.Scan(
			&item.ID, &item.UserID, &orgID, &item.Filename, &folderIDVal,
			&curVerID, &item.IsDeleted, &item.CreatedAt, &item.UpdatedAt, &sizeBytes,
		)
		if err != nil {
			return nil, err
		}

		if orgID.Valid {
			item.OrganizationID = &orgID.String
		}
		if folderIDVal.Valid {
			item.FolderID = &folderIDVal.String
		}
		if curVerID.Valid {
			item.CurrentVersionID = &curVerID.String
		}
		if sizeBytes.Valid {
			item.SizeBytes = &sizeBytes.Int64
		}

		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) GetByIDWithPhysical(ctx context.Context, id string) (*DownloadFile, error) {
	query := `
		SELECT f.id, f.filename, pf.storage_path, pf.compression_algorithm
		FROM files f
		JOIN file_versions fv ON fv.id = f.current_version_id
		JOIN physical_files pf ON pf.id = fv.physical_file_id
		WHERE f.id = $1 AND f.is_deleted = false`

	var file DownloadFile
	var alg sql.NullString
	err := r.db.QueryRow(ctx, query, id).Scan(&file.ID, &file.Filename, &file.StoragePath, &alg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if alg.Valid {
		file.CompressionAlgorithm = &alg.String
	}
	return &file, nil
}

func (r *Repository) MarkDeleted(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := tx.Exec(ctx, `UPDATE files SET is_deleted = true, updated_at = NOW() WHERE id = $1`, id)
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
		WHERE f.id = $1`

	var pf PhysicalFile
	var cs sql.NullInt64
	var ca sql.NullString
	var cRatio sql.NullFloat64

	err := tx.QueryRow(ctx, query, fileID).Scan(
		&pf.ID, &pf.HashSHA256, &pf.StoragePath, &pf.OriginalSize,
		&cs, &ca, &cRatio, &pf.ReferenceCount, &pf.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if cs.Valid {
		pf.CompressedSize = &cs.Int64
	}
	if ca.Valid {
		pf.CompressionAlgorithm = &ca.String
	}
	if cRatio.Valid {
		pf.CompressionRatio = &cRatio.Float64
	}
	return &pf, nil
}

func (r *Repository) FindByUserAndFilename(ctx context.Context, tx pgx.Tx, userID, filename string) (*File, error) {
	query := `
		SELECT id, user_id, organization_id, filename, folder_id, current_version_id, is_deleted, created_at, updated_at
		FROM files WHERE user_id = $1 AND filename = $2 AND is_deleted = false LIMIT 1`

	var file File
	var orgID, curVerID sql.NullString
	err := tx.QueryRow(ctx, query, userID, filename).Scan(
		&file.ID, &file.UserID, &orgID, &file.Filename, &file.FolderID,
		&curVerID, &file.IsDeleted, &file.CreatedAt, &file.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if orgID.Valid {
		file.OrganizationID = &orgID.String
	}
	if curVerID.Valid {
		file.CurrentVersionID = &curVerID.String
	}
	return &file, nil
}

func (r *Repository) GetLatestVersionNumber(ctx context.Context, tx pgx.Tx, fileID string) (int, error) {
	var v int
	err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version_number), 0) FROM file_versions WHERE file_id = $1`, fileID).Scan(&v)
	return v, err
}

func (r *Repository) GetVersionsByFileID(ctx context.Context, fileID string) ([]FileVersionInfo, error) {
	query := `
		SELECT fv.id, fv.version_number, fv.created_at,
		       pf.id, pf.original_size, pf.compressed_size, pf.compression_algorithm, pf.compression_ratio
		FROM file_versions fv
		JOIN physical_files pf ON pf.id = fv.physical_file_id
		WHERE fv.file_id = $1 ORDER BY fv.version_number DESC`

	rows, err := r.db.Query(ctx, query, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []FileVersionInfo
	for rows.Next() {
		var v FileVersionInfo
		var cs sql.NullInt64
		var alg sql.NullString
		var ratio sql.NullFloat64
		if err := rows.Scan(&v.ID, &v.VersionNumber, &v.CreatedAt,
			&v.PhysicalFileID, &v.OriginalSize, &cs, &alg, &ratio); err != nil {
			return nil, err
		}
		if cs.Valid {
			v.CompressedSize = &cs.Int64
		}
		if alg.Valid {
			v.CompressionAlgorithm = &alg.String
		}
		if ratio.Valid {
			v.CompressionRatio = &ratio.Float64
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func (r *Repository) GetVersionByNumber(ctx context.Context, tx pgx.Tx, fileID string, versionNumber int) (*FileVersion, error) {
	query := `SELECT id, file_id, physical_file_id, version_number, uploaded_by, created_at FROM file_versions WHERE file_id = $1 AND version_number = $2`
	var v FileVersion
	err := tx.QueryRow(ctx, query, fileID, versionNumber).Scan(&v.ID, &v.FileID, &v.PhysicalFileID, &v.VersionNumber, &v.UploadedBy, &v.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &v, nil
}

func (r *Repository) GetFileWithDetails(ctx context.Context, fileID string) (*FileDetails, error) {
	query := `
		SELECT f.id, f.user_id, f.organization_id, f.filename, f.folder_id, f.current_version_id, f.is_deleted, f.created_at, f.updated_at,
		       fv.id, fv.version_number, fv.uploaded_by, fv.created_at,
		       pf.id, pf.hash_sha256, pf.storage_path, pf.original_size, pf.compressed_size, pf.compression_algorithm, pf.compression_ratio, pf.reference_count, pf.created_at
		FROM files f
		LEFT JOIN file_versions fv ON fv.id = f.current_version_id
		LEFT JOIN physical_files pf ON pf.id = fv.physical_file_id
		WHERE f.id = $1 AND f.is_deleted = false`

	var details FileDetails
	var orgID, folderID, curVerID, verID, uploadedBy, physID sql.NullString
	var cs sql.NullInt64
	var alg sql.NullString
	var ratio sql.NullFloat64
	var verCreatedAt, physCreatedAt sql.NullTime

	err := r.db.QueryRow(ctx, query, fileID).Scan(
		&details.File.ID, &details.File.UserID, &orgID, &details.File.Filename, &folderID, &curVerID,
		&details.File.IsDeleted, &details.File.CreatedAt, &details.File.UpdatedAt,
		&verID, &details.CurrentVersion.Number, &uploadedBy, &verCreatedAt,
		&physID, &details.PhysicalFile.HashSHA256, &details.PhysicalFile.StoragePath,
		&details.PhysicalFile.OriginalSize, &cs, &alg, &ratio,
		&details.PhysicalFile.ReferenceCount, &physCreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if orgID.Valid {
		details.File.OrganizationID = &orgID.String
	}
	if folderID.Valid {
		details.File.FolderID = &folderID.String
	}
	if curVerID.Valid {
		details.File.CurrentVersionID = &curVerID.String
	}
	if verID.Valid {
		details.CurrentVersion.ID = verID.String
		details.CurrentVersion.HasData = true
		if uploadedBy.Valid {
			details.CurrentVersion.UploadedBy = uploadedBy.String
		}
		if verCreatedAt.Valid {
			details.CurrentVersion.CreatedAt = verCreatedAt.Time
		}
	}
	if physID.Valid {
		details.PhysicalFile.ID = physID.String
		details.PhysicalFile.HasData = true
		if cs.Valid {
			details.PhysicalFile.CompressedSize = &cs.Int64
		}
		if alg.Valid {
			details.PhysicalFile.CompressionAlgorithm = &alg.String
		}
		if ratio.Valid {
			details.PhysicalFile.CompressionRatio = &ratio.Float64
		}
		if physCreatedAt.Valid {
			details.PhysicalFile.CreatedAt = physCreatedAt.Time
		}
	}
	return &details, nil
}

// ==================== FOLDERS ====================

func (r *Repository) CreateFolder(ctx context.Context, tx pgx.Tx, folder *Folder) error {
	query := `INSERT INTO folders (id, user_id, parent_id, name, is_system, created_at) VALUES ($1, $2, $3, $4, $5, NOW())`
	_, err := tx.Exec(ctx, query, folder.ID, folder.UserID, folder.ParentID, folder.Name, folder.IsSystem)
	return err
}

func (r *Repository) GetFolderByID(ctx context.Context, tx pgx.Tx, id string) (*Folder, error) {
	query := `SELECT id, user_id, parent_id, name, is_system, created_at FROM folders WHERE id = $1`

	var f Folder
	var pID sql.NullString

	// 🔑 Используем хелпер вместо прямого tx.QueryRow
	err := r.queryRow(ctx, tx, query, id).Scan(
		&f.ID, &f.UserID, &pID, &f.Name, &f.IsSystem, &f.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if pID.Valid {
		f.ParentID = &pID.String
	}
	return &f, nil
}

func (r *Repository) GetFoldersByParent(ctx context.Context, tx pgx.Tx, userID string, parentID *string) ([]Folder, error) {
	query := `
        SELECT id, user_id, parent_id, name, is_system, created_at 
        FROM folders 
        WHERE user_id = $1 AND parent_id IS NOT DISTINCT FROM $2 
        ORDER BY name ASC
    `

	// 🔑 Используем хелпер
	rows, err := r.query(ctx, tx, query, userID, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 🔑 Инициализируем как пустой срез, а не nil
	folders := []Folder{}
	for rows.Next() {
		var f Folder
		var pID sql.NullString
		if err := rows.Scan(&f.ID, &f.UserID, &pID, &f.Name, &f.IsSystem, &f.CreatedAt); err != nil {
			return nil, err
		}
		if pID.Valid {
			f.ParentID = &pID.String
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

func (r *Repository) GetFolderWithChildren(ctx context.Context, tx pgx.Tx, folderID string) (*Folder, []Folder, []FileListItem, error) {
	folder, err := r.GetFolderByID(ctx, tx, folderID)
	if err != nil {
		return nil, nil, nil, err
	}
	subfolders, err := r.GetFoldersByParent(ctx, tx, folder.UserID, &folder.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	files, err := r.GetAllByUserID(ctx, folder.UserID, &folder.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	return folder, subfolders, files, nil
}

func (r *Repository) FolderExists(ctx context.Context, tx pgx.Tx, userID, parentID, name string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM folders WHERE user_id=$1 AND parent_id IS NOT DISTINCT FROM $2 AND name=$3)`

	var exists bool
	err := r.queryRow(ctx, tx, query, userID, parentID, name).Scan(&exists)
	return exists, err
}

func (r *Repository) GetOrCreateTrashFolder(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var trashID string
	query := `SELECT id FROM folders WHERE user_id = $1 AND name = $2 AND is_system = true`

	err := r.queryRow(ctx, tx, query, userID, "Корзина").Scan(&trashID)
	if err == nil {
		return trashID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	trashID = uuid.NewString()
	insertQuery := `INSERT INTO folders (id, user_id, parent_id, name, is_system, created_at) VALUES ($1, $2, NULL, $3, true, NOW())`

	_, err = r.exec(ctx, tx, insertQuery, trashID, userID, "Корзина")
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return r.GetOrCreateTrashFolder(ctx, tx, userID)
		}
		return "", err
	}
	return trashID, nil
}

func (r *Repository) MoveFilesToFolder(ctx context.Context, tx pgx.Tx, sourceFolderID, targetFolderID, userID string) error {
	_, err := tx.Exec(ctx, `UPDATE files SET folder_id = $1, updated_at = NOW() WHERE folder_id = $2 AND user_id = $3 AND is_deleted = false`, targetFolderID, sourceFolderID, userID)
	return err
}

func (r *Repository) MoveSubfoldersToFolder(ctx context.Context, tx pgx.Tx, sourceFolderID, targetFolderID, userID string) error {
	_, err := tx.Exec(ctx, `UPDATE folders SET parent_id = $1 WHERE parent_id = $2 AND user_id = $3`, targetFolderID, sourceFolderID, userID)
	return err
}

func (r *Repository) DeleteFolder(ctx context.Context, tx pgx.Tx, id string) error {
	var userID, name string
	var isSystem bool
	query := `SELECT user_id, name, is_system FROM folders WHERE id = $1`
	err := tx.QueryRow(ctx, query, id).Scan(&userID, &name, &isSystem)
	if err != nil {
		return err
	}
	if isSystem {
		return errors.New("cannot delete system folder")
	}

	trashID, err := r.GetOrCreateTrashFolder(ctx, tx, userID)
	if err != nil {
		return fmt.Errorf("failed to get trash folder: %w", err)
	}
	if err := r.MoveFilesToFolder(ctx, tx, id, trashID, userID); err != nil {
		return err
	}
	if err := r.MoveSubfoldersToFolder(ctx, tx, id, trashID, userID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM folders WHERE id = $1`, id)
	return err
}

func (r *Repository) GetTrashFiles(ctx context.Context, tx pgx.Tx, userID string) ([]FileListItem, error) {
	trashID, err := r.GetOrCreateTrashFolder(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	return r.GetAllByUserID(ctx, userID, &trashID)
}

// ==================== UTILS ====================

func scanFileListItems(rows pgx.Rows) ([]FileListItem, error) {
	defer rows.Close()
	var items []FileListItem
	for rows.Next() {
		var item FileListItem
		var orgID, folderID, curVerID sql.NullString
		if err := rows.Scan(&item.ID, &item.UserID, &orgID, &item.Filename, &folderID, &curVerID, &item.IsDeleted, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if orgID.Valid {
			item.OrganizationID = &orgID.String
		}
		if folderID.Valid {
			item.FolderID = &folderID.String
		}
		if curVerID.Valid {
			item.CurrentVersionID = &curVerID.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetFileByID получает файл по ID
func (r *Repository) GetFileByID(ctx context.Context, tx pgx.Tx, id string) (*File, error) {
	query := `SELECT id, user_id, organization_id, filename, folder_id, current_version_id, is_deleted, created_at, updated_at FROM files WHERE id = $1`
	row := r.queryRow(ctx, tx, query, id)

	var f File
	var orgID, folderID, curVerID sql.NullString
	err := row.Scan(&f.ID, &f.UserID, &orgID, &f.Filename, &folderID, &curVerID, &f.IsDeleted, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if orgID.Valid {
		f.OrganizationID = &orgID.String
	}
	if folderID.Valid {
		f.FolderID = &folderID.String
	}
	if curVerID.Valid {
		f.CurrentVersionID = &curVerID.String
	}
	return &f, nil
}

// MoveFileToFolder обновляет folder_id у файла
func (r *Repository) MoveFileToFolder(ctx context.Context, tx pgx.Tx, fileID string, folderID *string) error {
	_, err := r.exec(ctx, tx, `UPDATE files SET folder_id = $1, updated_at = NOW() WHERE id = $2`, folderID, fileID)
	return err
}

// MoveFolderToParent обновляет parent_id у папки
func (r *Repository) MoveFolderToParent(ctx context.Context, tx pgx.Tx, folderID string, parentID *string) error {
	_, err := r.exec(ctx, tx, `UPDATE folders SET parent_id = $1 WHERE id = $2`, parentID, folderID)
	return err
}

// IsDescendantOf проверяет, является ли potentialDescendant потомком ancestorID
func (r *Repository) IsDescendantOf(ctx context.Context, tx pgx.Tx, potentialDescendant, ancestorID string) (bool, error) {
	// Рекурсивный запрос: идём вверх от potentialDescendant, ищем ancestorID
	query := `
        WITH RECURSIVE ancestors AS (
            SELECT id, parent_id FROM folders WHERE id = $1
            UNION ALL
            SELECT f.id, f.parent_id FROM folders f
            INNER JOIN ancestors a ON f.id = a.parent_id
        )
        SELECT EXISTS(SELECT 1 FROM ancestors WHERE id = $2)
    `
	var exists bool
	err := r.queryRow(ctx, tx, query, potentialDescendant, ancestorID).Scan(&exists)
	return exists, err
}
