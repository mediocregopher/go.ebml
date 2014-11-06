package ebml

import (
	"bytes"
	"io"
	"os"
	. "testing"
)

func TestNumPrecedingZeros(t *T) {
	m := map[byte]byte{
		0x80: 0,
		0x40: 1,
		0x20: 2,
		0x10: 3,
		0x08: 4,
		0x04: 5,
		0x02: 6,
		0x01: 7,
		0x00: 8,
	}

	for in, out := range m {
		if n := numPrecedingZeros(in); n != out {
			t.Fatalf("(%x) %d != %d", in, n, out)
		}
	}
}

func sb(bs ...byte) string {
	b := make([]byte, 0, len(bs))
	b = append(b, bs...)
	return string(b)
}

func TestReadVarInt(t *T) {
	m := map[string]int64{
		sb(0x81):                                           0x01,
		sb(0xC1):                                           0x41,
		sb(0x40, 0x01):                                     0x01,
		sb(0x41, 0x21):                                     0x0121,
		sb(0x20, 0x41, 0x21):                               0x4121,
		sb(0x23, 0x41, 0x21):                               0x034121,
		sb(0x01, 0x41, 0x21, 0x12, 0x34, 0x56, 0x78, 0x9a): 0x4121123456789a,
	}

	for in, out := range m {
		e := RootElem(bytes.NewBuffer([]byte(in)))
		i, err := e.readVarInt()
		if err != nil {
			t.Fatal(err)
		}
		if i != out {
			t.Fatalf("(%x) %d != %d", in, i, out)
		}
	}
}

func TestIntElem(t *T) {
	m := map[string]int64{
		sb(0x80, 0x80):                   0,
		sb(0x80, 0x81, 0x01):             0x01,
		sb(0x80, 0x82, 0x02, 0x01):       0x0201,
		sb(0x80, 0x83, 0x03, 0x02, 0x01): 0x030201,
	}
	for in, out := range m {
		b := []byte(in)
		e, err := RootElem(bytes.NewBuffer(b)).Next()
		if err != nil {
			t.Fatal(err)
		}

		i, err := e.Int()
		if err != nil {
			t.Fatal(err)
		} else if i != out {
			t.Fatalf("(%x) %x != %x", in, i, out)
		}

		ui, err := e.Uint()
		if err != nil {
			t.Fatal(err)
		} else if ui != uint64(out) {
			t.Fatalf("(%x) %x != %x", in, ui, out)
		}
	}
}

func TestStringElem(t *T) {
	m := map[string]string{
		sb(0x80, 0x80):                      "",
		sb(0x80, 0x83, 'f', 'o', 'o'):       "foo",
		sb(0x80, 0x85, 'f', 'o', 'o', 0, 0): "foo",
	}
	for in, out := range m {
		b := []byte(in)
		e, err := RootElem(bytes.NewBuffer(b)).Next()
		if err != nil {
			t.Fatal(err)
		}

		i, err := e.String()
		if err != nil {
			t.Fatal(err)
		} else if i != out {
			t.Fatalf("(%q) %q != %q", in, i, out)
		}
	}
}

func getTestFile(t *T) io.ReadCloser {
	f, err := os.Open("test.webm")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFile1(t *T) {
	f := getTestFile(t)
	defer f.Close()

	e := RootElem(f)

	if id, err := e.readVarInt(); err != nil {
		t.Fatal(err)
	} else if id != 0x0a45dfa3 {
		t.Fatalf("id isn't right: %x", id)
	}

	if size, err := e.readVarInt(); err != nil {
		t.Fatal(err)
	} else if size != 0x23 {
		t.Fatalf("id isn't right: %x", size)
	}

}