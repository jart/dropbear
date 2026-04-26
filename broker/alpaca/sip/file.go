package sip

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// File provides zero-copy iteration over sip message file.
// These files don't necessarily have strictly monotonic timestamps.
type File struct {
	path  string
	data  []byte
	count int
	pos   int
}

// OpenFile opens a sip message file for reading.
// The returned File is guaranteed to have at least one message.
// The file format is raw 64-byte messages with no length prefix.
func OpenFile(path string) (*File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := int(info.Size())
	count := size / barSize
	if count == 0 || size%barSize != 0 {
		return nil, errors.New("invalid message file size")
	}
	data, err := syscall.Mmap(int(file.Fd()), 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	return &File{
		path:  path,
		data:  data,
		count: count,
	}, nil
}

// String returns a string representation of object.
func (r *File) String() string {
	return "File{" + r.path + "}"
}

// Close closes the file.
func (r *File) Close() error {
	err := syscall.Munmap(r.data)
	r.data = nil
	return err
}

// Count returns the number of bars in the file.
func (r *File) Count() int {
	return r.count
}

// Get returns a pointer to the message at the given index.
func (r *File) Get(index int) *Message {
	return (*Message)(unsafe.Pointer(&r.data[index*barSize]))
}

// EOF returns whether we have reached the end of the file.
func (r *File) EOF() bool {
	return r.pos == r.count
}

// Read returns a pointer to the next message in the file.
// Returns nil when there are no more messages.
func (r *File) Read() *Message {
	if r.pos == r.count {
		return nil
	}
	msg := r.Get(r.pos)
	r.pos++
	return msg
}

// Peek returns a pointer to the next message without advancing the position.
// Returns nil when there are no more messages.
func (r *File) Peek() *Message {
	if r.pos == r.count {
		return nil
	}
	return r.Get(r.pos)
}

const barSize = int(unsafe.Sizeof(Message{}))
