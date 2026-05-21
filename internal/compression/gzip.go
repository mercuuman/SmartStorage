package compression

import (
	"compress/gzip"
	"io"
)

type GzipCompressor struct{}

func NewGzipCompressor() *GzipCompressor {
	return &GzipCompressor{}
}

func (g *GzipCompressor) Name() string {
	return "gzip"
}

func (g *GzipCompressor) Compress(
	r io.Reader,
	w io.Writer,
) error {

	gw := gzip.NewWriter(w)

	defer gw.Close()

	_, err := io.Copy(gw, r)

	return err
}

func (g *GzipCompressor) Decompress(
	r io.Reader,
	w io.Writer,
) error {

	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}

	defer gr.Close()

	_, err = io.Copy(w, gr)

	return err
}
