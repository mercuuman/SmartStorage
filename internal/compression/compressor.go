package compression

import "io"

type Compressor interface {
	Name() string

	Compress(
		r io.Reader,
		w io.Writer,
	) error

	Decompress(
		r io.Reader,
		w io.Writer,
	) error
}
