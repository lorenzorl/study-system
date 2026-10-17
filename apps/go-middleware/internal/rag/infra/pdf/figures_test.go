package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/infra/pdf"
)

// realImageTable is the exact pdfimages -list shape produced by poppler 25.11.0
// for the fixture: one image on page 2 and one on page 3.
const realImageTable = `page   num  type   width height color comp bpc  enc interp  object ID x-ppi y-ppi size ratio
--------------------------------------------------------------------------------------------
   2     0 image       2     2  gray    1   8  image  no         7  0     1     1   16B 400%
   3     1 image       2     2  gray    1   8  image  no         7  0 0.720 0.720   16B 400%
`

func TestFigureIndex_CountsTheRealTable(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte(realImageTable), nil
	}}

	index, err := pdf.NewFigureIndex("book.pdf", stub.runner())
	require.NoError(t, err)

	counts, err := index.ImageCounts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[int]int{2: 1, 3: 1}, counts)
}

// TestFigureIndex_IsHeaderDriven moves the columns around and checks the row is
// still read correctly. A parser that hardcoded positions would pass the real
// table and fail here.
func TestFigureIndex_IsHeaderDriven(t *testing.T) {
	reordered := `num page width type height color comp bpc enc interp object ID x-ppi y-ppi size ratio
-------------------------------------------------------------------------------------------
  0   2     2 image      2  gray    1   8 image  no        7  0     1     1   16B 400%
`
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte(reordered), nil
	}}

	index, err := pdf.NewFigureIndex("book.pdf", stub.runner())
	require.NoError(t, err)

	counts, err := index.ImageCounts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[int]int{2: 1}, counts)
}

func TestFigureIndex_IgnoresMasks(t *testing.T) {
	table := `page   num  type   width height color comp bpc  enc interp  object ID x-ppi y-ppi size ratio
--------------------------------------------------------------------------------------------
   2     0 image       2     2  gray    1   8  image  no         7  0     1     1   16B 400%
   2     1 smask       2     2  gray    1   8  image  no         8  0     1     1   16B 400%
`
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte(table), nil
	}}

	index, err := pdf.NewFigureIndex("book.pdf", stub.runner())
	require.NoError(t, err)

	counts, err := index.ImageCounts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[int]int{2: 1}, counts, "a mask is not a picture and must not count")
}

func TestFigureIndex_SumsMultipleImagesOnOnePage(t *testing.T) {
	table := `page   num  type   width height color comp bpc  enc interp  object ID x-ppi y-ppi size ratio
--------------------------------------------------------------------------------------------
   1     0 image       2     2  gray    1   8  image  no         7  0     1     1   16B 400%
   1     1 image       3     3  gray    1   8  image  no         8  0     1     1   16B 400%
`
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte(table), nil
	}}

	index, err := pdf.NewFigureIndex("book.pdf", stub.runner())
	require.NoError(t, err)

	counts, err := index.ImageCounts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[int]int{1: 2}, counts)
}

func TestFigureIndex_EmptyOutputMeansNoImages(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return nil, nil
	}}

	index, err := pdf.NewFigureIndex("book.pdf", stub.runner())
	require.NoError(t, err)

	counts, err := index.ImageCounts(t.Context())
	require.NoError(t, err)
	assert.Empty(t, counts, "a book with no diagrams is not an error")
}

func TestFigureIndex_HeaderWithNoRowsMeansNoImages(t *testing.T) {
	table := "page   num  type   width height color comp bpc  enc interp  object ID x-ppi y-ppi size ratio\n" +
		"--------------------------------------------------------------------------------------------\n"
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte(table), nil
	}}

	index, err := pdf.NewFigureIndex("book.pdf", stub.runner())
	require.NoError(t, err)

	counts, err := index.ImageCounts(t.Context())
	require.NoError(t, err)
	assert.Empty(t, counts)
}

// TestFigureIndex_UnparseableRowIsAnError pins the failure mode that must not be
// silent: a row that cannot be read is reported with the offending line, because
// counting it as zero would be indistinguishable from a book with no diagrams.
func TestFigureIndex_UnparseableRowIsAnError(t *testing.T) {
	table := `page   num  type   width height color comp bpc  enc interp  object ID x-ppi y-ppi size ratio
--------------------------------------------------------------------------------------------
 not-a-page     0 image       2     2  gray    1   8  image  no         7  0     1     1   16B 400%
`
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte(table), nil
	}}

	index, err := pdf.NewFigureIndex("book.pdf", stub.runner())
	require.NoError(t, err)

	_, err = index.ImageCounts(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-page", "the error must name the line it could not parse")
}

func TestFigureIndex_MissingHeaderIsAnError(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte("this is not pdfimages output\n"), nil
	}}

	index, err := pdf.NewFigureIndex("book.pdf", stub.runner())
	require.NoError(t, err)

	_, err = index.ImageCounts(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "header")
}
