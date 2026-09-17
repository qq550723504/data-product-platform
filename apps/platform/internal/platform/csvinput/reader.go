// Package csvinput handles the optional UTF-8 BOM at the CSV reader boundary.
// It does not alter uploaded object bytes or their checksums.
package csvinput

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"io"
)

func NewReader(source io.Reader) *csv.Reader {
	buffered := bufio.NewReader(source)
	if prefix, err := buffered.Peek(3); err == nil && bytes.Equal(prefix, []byte{0xef, 0xbb, 0xbf}) {
		_, _ = buffered.Discard(3)
	}
	return csv.NewReader(buffered)
}
