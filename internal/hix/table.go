package hix

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Table là bảng text căn cột đơn giản: header + các hàng.
type Table struct {
	Header []string
	Rows   [][]string
}

// NewTable tạo bảng với header cho trước.
func NewTable(header ...string) *Table {
	return &Table{Header: append([]string(nil), header...)}
}

// Add thêm một hàng vào bảng.
func (t *Table) Add(cols ...string) {
	t.Rows = append(t.Rows, append([]string(nil), cols...))
}

// Render in bảng ra w (dùng text/tabwriter để căn cột).
func (t *Table) Render(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if len(t.Header) > 0 {
		if _, err := fmt.Fprintln(tw, strings.Join(t.Header, "\t")); err != nil {
			return err
		}
	}
	for _, row := range t.Rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}
