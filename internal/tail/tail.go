// Package tail reads a growing line-oriented file incrementally.
package tail

import (
	"bytes"
	"errors"
	"io"
	"os"
)

// ErrGone reports a file that no longer exists.
var ErrGone = errors.New("file is gone")

// Follower returns the complete lines appended to a file since the last Read.
// A trailing line without its newline is held until the newline arrives.
type Follower struct {
	path    string
	offset  int64
	info    os.FileInfo
	partial []byte
}

func New(path string) *Follower {
	return &Follower{path: path}
}

func (f *Follower) Path() string { return f.path }

// Read returns new complete lines. reset is true when the file was truncated
// or replaced, in which case lines start from the beginning of the new file
// and the caller should drop what it built from the old one.
func (f *Follower) Read() (lines [][]byte, reset bool, err error) {
	info, err := os.Stat(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, ErrGone
	}
	if err != nil {
		return nil, false, err
	}
	if f.info != nil && (!os.SameFile(f.info, info) || info.Size() < f.offset) {
		f.offset, f.partial, reset = 0, nil, true
	}
	f.info = info
	if info.Size() == f.offset {
		return nil, reset, nil
	}
	file, err := os.Open(f.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, reset, ErrGone
		}
		return nil, reset, err
	}
	defer file.Close()
	if _, err := file.Seek(f.offset, io.SeekStart); err != nil {
		return nil, reset, err
	}
	chunk, err := io.ReadAll(io.LimitReader(file, info.Size()-f.offset))
	if err != nil {
		return nil, reset, err
	}
	f.offset += int64(len(chunk))
	data := chunk
	if len(f.partial) > 0 {
		data = append(f.partial, chunk...)
		f.partial = nil
	}
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		if line := bytes.TrimRight(data[:i], "\r"); len(line) > 0 {
			lines = append(lines, line)
		}
		data = data[i+1:]
	}
	if len(data) > 0 {
		f.partial = append([]byte(nil), data...)
	}
	return lines, reset, nil
}
