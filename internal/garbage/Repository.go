package garbage

import (
	"context"
	"os"
)

func (s *Service) Run(ctx context.Context) error {

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. найти все физические файлы с ref_count = 0
	query := `
		SELECT id, storage_path
		FROM physical_files
		WHERE reference_count <= 0
	`

	rows, err := tx.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	type item struct {
		id   string
		path string
	}

	var toDelete []item

	for rows.Next() {
		var i item
		if err := rows.Scan(&i.id, &i.path); err != nil {
			return err
		}
		toDelete = append(toDelete, i)
	}

	// 2. удалить с диска
	for _, f := range toDelete {
		_ = os.Remove(f.path)
	}

	// 3. удалить из БД
	for _, f := range toDelete {
		_, err := tx.Exec(ctx, `
			DELETE FROM physical_files
			WHERE id = $1
		`, f.id)

		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
