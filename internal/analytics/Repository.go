package analytics

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}
func (r *Repository) GetSystemStats(ctx context.Context) (*SystemStats, error) {

	query := `
		SELECT
			(SELECT COUNT(*) FROM files WHERE is_deleted = false) as total_files,
			(SELECT COUNT(*) FROM file_versions) as total_versions,
			(SELECT COUNT(*) FROM physical_files) as total_physical_files,

			COALESCE(SUM(original_size), 0),
			COALESCE(SUM(COALESCE(compressed_size, original_size)), 0)

		FROM physical_files;
	`

	var stats SystemStats

	err := r.db.QueryRow(ctx, query).Scan(
		&stats.TotalFiles,
		&stats.TotalVersions,
		&stats.TotalPhysicalFiles,
		&stats.TotalOriginalSize,
		&stats.TotalCompressedSize,
	)

	if err != nil {
		return nil, err
	}

	// вычисления в Go
	if stats.TotalOriginalSize > 0 {
		saved := stats.TotalOriginalSize - stats.TotalCompressedSize

		stats.SpaceSavedBytes = saved
		stats.SpaceSavedPercent =
			float64(saved) / float64(stats.TotalOriginalSize) * 100
	}

	return &stats, nil
}

func (r *Repository) GetUserStats(ctx context.Context, userID string) (*UserStats, error) {

	query := `
		SELECT
			COUNT(DISTINCT f.id) as total_files,
			COUNT(fv.id) as total_versions,

			COALESCE(SUM(pf.original_size), 0),
			COALESCE(SUM(COALESCE(pf.compressed_size, pf.original_size)), 0),

			COUNT(CASE WHEN pf.reference_count > 1 THEN 1 END)

		FROM files f
		LEFT JOIN file_versions fv ON fv.file_id = f.id
		LEFT JOIN physical_files pf ON pf.id = fv.physical_file_id
		WHERE f.user_id = $1
		  AND f.is_deleted = false;
	`

	var s UserStats
	s.UserID = userID

	err := r.db.QueryRow(ctx, query, userID).Scan(
		&s.TotalFiles, &s.TotalVersions, &s.TotalOriginalSize, &s.TotalCompressedSize, &s.TotalDeduplicatedFiles,
	)

	if err != nil {
		return nil, err
	}

	if s.TotalOriginalSize > 0 {
		s.SpaceSavedBytes =
			s.TotalOriginalSize - s.TotalCompressedSize

		s.SpaceSavedPercent =
			float64(s.SpaceSavedBytes) /
				float64(s.TotalOriginalSize) * 100
	}

	return &s, nil
}

func (r *Repository) GetCompressionStats(ctx context.Context) ([]CompressionStats, error) {

	query := `
		SELECT
			COALESCE(compression_algorithm, 'none') as algorithm,

			COUNT(*) as files_count,

			COALESCE(SUM(original_size), 0),
			COALESCE(SUM(COALESCE(compressed_size, original_size)), 0),

			COALESCE(AVG(
				CASE
					WHEN compressed_size IS NULL THEN 1
					ELSE (compressed_size::float / original_size)
				END
			), 1)

		FROM physical_files
		GROUP BY compression_algorithm
		ORDER BY files_count DESC;
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CompressionStats

	for rows.Next() {

		var s CompressionStats

		err := rows.Scan(
			&s.Algorithm,
			&s.FilesCount,
			&s.TotalOriginalSize,
			&s.TotalCompressedSize,
			&s.AvgCompressionRatio,
		)

		if err != nil {
			return nil, err
		}

		result = append(result, s)
	}

	return result, nil
}
