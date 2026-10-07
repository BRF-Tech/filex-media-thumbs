// Package reader reads one untrusted file under limits: a budget of bytes,
// a deadline, and a cap on how often the file is read again from its start.
//
// filex hands an app a file as a stream (file_open / file_read on the
// call's one reference): there are no ranged reads. A Source keeps a
// position over that stream. A jump forward is read and dropped; a jump back
// opens the file again from its start. Every byte that comes off the file
// counts against the budget - a skipped byte was read too - and every read
// checks the deadline, so a file is given up with ErrTooLarge or ErrTimeout
// rather than read for ever. filex's own limits (the largest file it sends,
// the time per call, the memory) stand behind these; these turn a file that
// is too much into a sentence instead of a killed instance.
package reader

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"
)

var (
	// ErrTooLarge: the file asks for more than the limits allow (its own
	// lengths, or the bytes it takes to reach what is wanted).
	ErrTooLarge = errors.New("past the reading limits")
	// ErrTimeout: past the deadline. Not the file's fault, necessarily: a
	// slow storage ends here too.
	ErrTimeout = errors.New("past the time limit")
	// ErrEndsEarly: the file ends before what it said it holds. Wrapped with
	// the format's name.
	ErrEndsEarly = errors.New("the file ends early")
)

// Opener opens the file from its start.
type Opener func() (io.ReadCloser, error)

// MaxOpens is how many times a Source opens its file. A reader that keeps
// jumping back is going round in circles over a hostile file.
const MaxOpens = 8

// Source is one file, read at a position.
type Source struct {
	open     Opener
	size     int64 // the file's length; -1 when unknown
	budget   int64
	deadline time.Time
	pos      int64 // where the next Read reads from
	body     io.ReadCloser
	bodyPos  int64 // where body is
	opens    int
}

// New reads the file open opens, of size bytes (-1: unknown), under budget
// bytes and until deadline (zero: none).
func New(open Opener, size, budget int64, deadline time.Time) *Source {
	if size < 0 {
		size = -1
	}
	return &Source{open: open, size: size, budget: budget, deadline: deadline}
}

// FromBytes reads b: what a test hands a reader.
func FromBytes(b []byte, budget int64) *Source {
	return New(func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(b)), nil }, int64(len(b)), budget, time.Time{})
}

// Size is the file's length, -1 when unknown.
func (s *Source) Size() int64 { return s.size }

// Pos is where the next Read reads from.
func (s *Source) Pos() int64 { return s.pos }

// Budget is what is left of the bytes the Source may read.
func (s *Source) Budget() int64 { return s.budget }

// Opens is how many times the file was opened.
func (s *Source) Opens() int { return s.opens }

// SeekTo sets the position the next Read reads from. (Not Seek: io.Seeker's
// signature is (int64, int) (int64, error), and vet holds a method of that name
// to it.)
func (s *Source) SeekTo(off int64) { s.pos = off }

// Skip moves the position n bytes forward.
func (s *Source) Skip(n int64) { s.pos += n }

// Late reports whether the deadline has passed.
func (s *Source) Late() bool {
	return !s.deadline.IsZero() && time.Now().After(s.deadline)
}

func (s *Source) Read(p []byte) (int, error) {
	if s.Late() {
		return 0, ErrTimeout
	}
	if s.pos < 0 {
		return 0, fmt.Errorf("reader: a negative position (%d)", s.pos)
	}
	if s.size >= 0 && s.pos >= s.size {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	if err := s.reach(); err != nil {
		return 0, err
	}
	if s.budget <= 0 {
		return 0, ErrTooLarge
	}
	if int64(len(p)) > s.budget {
		p = p[:s.budget]
	}
	n, err := s.body.Read(p)
	s.budget -= int64(n)
	s.pos += int64(n)
	s.bodyPos = s.pos
	return n, err
}

// reach leaves an open body at the position.
func (s *Source) reach() error {
	if s.body != nil && s.bodyPos == s.pos {
		return nil
	}
	if s.body == nil || s.bodyPos > s.pos {
		s.closeBody()
		if s.opens >= MaxOpens {
			return fmt.Errorf("reader: the file was read from its start %d times: %w", s.opens, ErrTooLarge)
		}
		rc, err := s.open()
		if err != nil {
			return err
		}
		s.opens++
		s.body, s.bodyPos = rc, 0
	}
	return s.drop(s.pos - s.bodyPos)
}

// drop reads n bytes of the open body and throws them away.
func (s *Source) drop(n int64) error {
	var scratch [32 << 10]byte
	for n > 0 {
		if s.Late() {
			return ErrTimeout
		}
		k := int64(len(scratch))
		if k > n {
			k = n
		}
		if k > s.budget {
			return ErrTooLarge
		}
		got, err := s.body.Read(scratch[:k])
		s.budget -= int64(got)
		s.bodyPos += int64(got)
		n -= int64(got)
		if errors.Is(err, io.EOF) {
			if n > 0 {
				return ErrEndsEarly
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Source) closeBody() {
	if s.body != nil {
		_ = s.body.Close()
		s.body = nil
	}
}

// Close releases the open body.
func (s *Source) Close() { s.closeBody() }

// ReadAt reads exactly n bytes at off. A file that ends first is damaged
// (what names the format in the error) - unless a limit or the clock
// stopped the read.
func (s *Source) ReadAt(off int64, n int, what string) ([]byte, error) {
	s.SeekTo(off)
	return s.ReadN(n, what)
}

// ReadN reads exactly n bytes at the position.
func (s *Source) ReadN(n int, what string) ([]byte, error) {
	if n < 0 {
		return nil, fmt.Errorf("%s: a negative length", what)
	}
	if int64(n) > s.budget {
		return nil, ErrTooLarge
	}
	b := make([]byte, n)
	if err := s.Fill(b, what); err != nil {
		return nil, err
	}
	return b, nil
}

// Fill reads len(b) bytes at the position into b.
func (s *Source) Fill(b []byte, what string) error {
	if int64(len(b)) > s.budget {
		return ErrTooLarge
	}
	if _, err := io.ReadFull(s, b); err != nil {
		if errors.Is(err, ErrTooLarge) || errors.Is(err, ErrTimeout) {
			return err
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, ErrEndsEarly) {
			return fmt.Errorf("%s: %w", what, ErrEndsEarly)
		}
		return err
	}
	return nil
}

// ReadAll reads the whole file from its start, at most limit bytes
// (ErrTooLarge past it).
func (s *Source) ReadAll(limit int64) ([]byte, error) {
	if s.size > limit {
		return nil, ErrTooLarge
	}
	s.SeekTo(0)
	b, err := io.ReadAll(io.LimitReader(s, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, ErrTooLarge
	}
	return b, nil
}

// BE32 reads a big-endian unsigned 32-bit number at the position.
func (s *Source) BE32(what string) (int64, error) {
	b, err := s.ReadN(4, what)
	if err != nil {
		return 0, err
	}
	return int64(b[0])<<24 | int64(b[1])<<16 | int64(b[2])<<8 | int64(b[3]), nil
}
