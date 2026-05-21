package compression

import (
	"bytes"
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
