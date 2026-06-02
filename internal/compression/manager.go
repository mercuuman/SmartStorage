package compression

import (
	"bytes"
	"errors"
	"io"
)

type Manager struct {
	compressors []Compressor
}

func NewManager() *Manager {
	return &Manager{
		compressors: []Compressor{
			NewGzipCompressor(),
			NewZstdCompressor(),
		},
	}
}

func (m *Manager) SelectBest(
	data []byte,
) (Compressor, []byte, error) {

	var bestCompressor Compressor
	var bestData []byte

	bestSize := -1

	for _, compressor := range m.compressors {

		var buf bytes.Buffer

		err := compressor.Compress(
			bytes.NewReader(data),
			&buf,
		)

		if err != nil {
			continue
		}

		size := buf.Len()

		if bestSize == -1 || size < bestSize {
			bestSize = size
			bestCompressor = compressor
			bestData = buf.Bytes()
		}
	}

	return bestCompressor, bestData, nil
}

func (m *Manager) GetByName(name string) Compressor {

	for _, c := range m.compressors {
		if c.Name() == name {
			return c
		}
	}

	return nil
}

// SelectBestForStream сжимает данные из reader в writer (streaming)
// Возвращает выбранный компрессор
func (m *Manager) SelectBestForStream(src io.Reader, dst io.Writer) (Compressor, error) {
	// Используем первый доступный компрессор (или можно тестировать все)
	if len(m.compressors) == 0 {
		return nil, errors.New("no compressors available")
	}

	compressor := m.compressors[0] // Возьми первый компрессор

	// Сжимаем потоково
	if err := compressor.Compress(src, dst); err != nil {
		return nil, err
	}

	return compressor, nil
}
