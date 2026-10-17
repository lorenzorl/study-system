//go:build ignore

// Command mkpdf writes a minimal three-page PDF used as a test fixture.
//
// Page 1 carries text only. Page 2 carries text and a 2x2 grayscale image.
// Page 3 carries only the image, which is the scanned-page case. The xref
// offsets are computed as the objects are emitted, so this is a well-formed PDF
// rather than one poppler has to repair.
//
// Run it from apps/go-middleware:
//
//	go run testdata/make_fixture.go testdata/three-pages.pdf
package main

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"strconv"
)

func main() {
	out := os.Args[1]
	data := build()
	if err := os.WriteFile(out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", out, len(data), "bytes")
}

func build() []byte {
	var buf bytes.Buffer

	// A 2x2 grayscale image: two dark pixels, two light ones.
	image := compress([]byte{0x00, 0x00, 0xff, 0xff})

	objects := []any{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 5 0 R 9 0 R] /Count 3 >>",
		page(6),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		pageWithImage(8),
		stream("BT /F1 24 Tf 72 700 Td (Page one text) Tj ET"),
		raw(fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 2 /Height 2 "+
			"/ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n", len(image)),
			image, "\nendstream"),
		stream("BT /F1 24 Tf 72 700 Td (Page two text) Tj ET\nq 100 0 0 100 72 400 cm /Im1 Do Q"),
		pageWithImage(10),
		stream("q 200 0 0 200 72 400 cm /Im1 Do Q"),
	}

	buf.WriteString("%PDF-1.4\n")

	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		number := index + 1
		offsets[number] = buf.Len()

		fmt.Fprintf(&buf, "%d 0 obj\n", number)
		switch value := object.(type) {
		case string:
			buf.WriteString(value)
		case []byte:
			buf.Write(value)
		default:
			panic("unreachable")
		}
		buf.WriteString("\nendobj\n")
	}

	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for number := 1; number <= len(objects); number++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[number])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(objects)+1, xref)

	return buf.Bytes()
}

// page is a text-only page whose content stream is the object with that number.
func page(contents int) string {
	return fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "+
		"/Resources << /Font << /F1 4 0 R >> >> /Contents %d 0 R >>", contents)
}

// pageWithImage additionally exposes the image as /Im1.
func pageWithImage(contents int) string {
	return fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "+
		"/Resources << /Font << /F1 4 0 R >> /XObject << /Im1 7 0 R >> >> /Contents %d 0 R >>", contents)
}

func stream(content string) string {
	return "<< /Length " + strconv.Itoa(len(content)) + " >>\nstream\n" + content + "\nendstream"
}

// raw concatenates parts as bytes, for the stream that is not text.
func raw(parts ...any) []byte {
	var out bytes.Buffer

	for _, part := range parts {
		switch value := part.(type) {
		case string:
			out.WriteString(value)
		case []byte:
			out.Write(value)
		}
	}
	return out.Bytes()
}

func compress(data []byte) []byte {
	var out bytes.Buffer

	writer := zlib.NewWriter(&out)
	if _, err := writer.Write(data); err != nil {
		panic(err)
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}
	return out.Bytes()
}
