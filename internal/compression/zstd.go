package compression

import (
	"io"

	"github.com/klauspost/compress/zstd"
)

type ZstdCompressor struct{}

func NewZstdCompressor() *ZstdCompressor {
	return &ZstdCompressor{}
}

func (z *ZstdCompressor) Name() string {
	return "zstd"
}

func (z *ZstdCompressor) Compress(
	r io.Reader,
	w io.Writer,
) error {

	encoder, err := zstd.NewWriter(w)
	if err != nil {
		return err
	}

	defer encoder.Close()

	_, err = io.Copy(encoder, r)

	return err
}

func (z *ZstdCompressor) Decompress(
	r io.Reader,
	w io.Writer,
) error {

	decoder, err := zstd.NewReader(r)
	if err != nil {
		return err
	}

	defer decoder.Close()

	_, err = io.Copy(w, decoder)

	return err
}
