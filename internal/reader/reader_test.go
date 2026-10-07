package reader

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// counting opens b and counts the opens and the bytes that come off it.
type counting struct {
	b     []byte
	opens int
	read  int64
}

func (c *counting) open() (io.ReadCloser, error) {
	c.opens++
	return io.NopCloser(&countReader{r: bytes.NewReader(c.b), n: &c.read}), nil
}

type countReader struct {
	r *bytes.Reader
	n *int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	*c.n += int64(n)
	return n, err
}

var alphabet = []byte("0123456789abcdefghijklmnopqrstuvwxyz")

func TestSource_ReadsAtPositions(t *testing.T) {
	c := &counting{b: alphabet}
	src := New(c.open, int64(len(alphabet)), 1<<20, time.Time{})
	defer src.Close()

	got, err := src.ReadAt(10, 5, "t")
	if err != nil || string(got) != "abcde" {
		t.Fatalf("ReadAt(10, 5) = %q, %v", got, err)
	}
	got, err = src.ReadAt(20, 3, "t")
	if err != nil || string(got) != "klm" {
		t.Fatalf("a jump forward: %q, %v", got, err)
	}
	if c.opens != 1 {
		t.Fatalf("a jump forward reads on through the open stream: %d opens", c.opens)
	}
	got, err = src.ReadAt(2, 3, "t")
	if err != nil || string(got) != "234" {
		t.Fatalf("a jump back: %q, %v", got, err)
	}
	if c.opens != 2 {
		t.Fatalf("a jump back opens the file again: %d opens", c.opens)
	}
	if src.Pos() != 5 {
		t.Fatalf("Pos after reading 3 at 2 = %d", src.Pos())
	}
}

func TestSource_AFileThatEndsEarlyIsDamaged(t *testing.T) {
	src := FromBytes(alphabet, 1<<20)
	_, err := src.ReadAt(30, 10, "fmt")
	if !errors.Is(err, ErrEndsEarly) || !strings.HasPrefix(err.Error(), "fmt:") {
		t.Fatalf("reading past the end: %v, want fmt: %v", err, ErrEndsEarly)
	}
	_, err = src.ReadAt(100, 1, "fmt")
	if !errors.Is(err, ErrEndsEarly) {
		t.Fatalf("reading from past the end: %v", err)
	}
}

// Every byte that comes off the file counts, the skipped ones too: a stream
// has to read a skip to get past it.
func TestSource_TheBudgetCountsSkippedBytes(t *testing.T) {
	src := FromBytes(alphabet, 8)
	if _, err := src.ReadAt(20, 4, "t"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("a read past the budget: %v, want ErrTooLarge", err)
	}
	src = FromBytes(alphabet, 8)
	if _, err := src.ReadN(9, "t"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("a length over the budget is refused before it is read: %v", err)
	}
	src = FromBytes(alphabet, 8)
	if _, err := src.ReadAt(2, 4, "t"); err != nil {
		t.Fatalf("2 skipped + 4 read is within 8: %v", err)
	}
	if src.Budget() != 2 {
		t.Fatalf("budget left %d, want 2", src.Budget())
	}
}

func TestSource_ReadAll(t *testing.T) {
	src := FromBytes(alphabet, 1<<20)
	b, err := src.ReadAll(64)
	if err != nil || !bytes.Equal(b, alphabet) {
		t.Fatalf("ReadAll = %q, %v", b, err)
	}
	if _, err := FromBytes(alphabet, 1<<20).ReadAll(10); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("a file over the limit: %v", err)
	}
	// Size unknown: the limit is found while reading.
	unknown := New(func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(alphabet)), nil }, -1, 1<<20, time.Time{})
	if _, err := unknown.ReadAll(10); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("a file of unknown size over the limit: %v", err)
	}
	if unknown.Size() != -1 {
		t.Fatalf("Size = %d, want -1", unknown.Size())
	}
}

func TestSource_PastTheDeadline(t *testing.T) {
	src := New(func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(alphabet)), nil }, int64(len(alphabet)), 1<<20, time.Now().Add(-time.Second))
	if _, err := src.ReadAt(0, 4, "t"); !errors.Is(err, ErrTimeout) {
		t.Fatalf("a read after the deadline: %v, want ErrTimeout", err)
	}
	if !src.Late() {
		t.Fatal("Late is false after the deadline")
	}
}

// A reader that keeps going back to the start of a hostile file is stopped.
func TestSource_OpensAreCapped(t *testing.T) {
	c := &counting{b: alphabet}
	src := New(c.open, int64(len(alphabet)), 1<<20, time.Time{})
	var err error
	for i := 0; i < MaxOpens+2 && err == nil; i++ {
		_, err = src.ReadAt(30, 1, "t")
		if err == nil {
			_, err = src.ReadAt(0, 1, "t")
		}
	}
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("going back and forth: %v, want ErrTooLarge", err)
	}
	if c.opens != MaxOpens {
		t.Fatalf("%d opens, want %d", c.opens, MaxOpens)
	}
}

func TestSource_BE32(t *testing.T) {
	src := FromBytes([]byte{0x01, 0x02, 0x03, 0x04, 0xFF}, 64)
	v, err := src.BE32("t")
	if err != nil || v != 0x01020304 {
		t.Fatalf("BE32 = %#x, %v", v, err)
	}
	if _, err := src.BE32("t"); !errors.Is(err, ErrEndsEarly) {
		t.Fatalf("BE32 past the end: %v", err)
	}
}
