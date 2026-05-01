package sip

import (
	"dropbear/clocky"
	"encoding/binary"
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// File provides zero-copy iteration over sip message file.
// These files don't necessarily have strictly monotonic timestamps.
type File struct {
	path    string
	data    []byte
	version int
	count   int
	pos     int
}

// OpenFile opens a sip message file for reading.
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
	if size < 4 {
		return nil, errors.New("invalid message file size")
	}
	data, err := syscall.Mmap(int(file.Fd()), 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	var version, count int
	if data[1] == 'S' && data[2] == 'I' && data[3] == 'P' && data[0] >= 128+2 {
		version = int(data[0]) - 128
		if version != 2 {
			syscall.Munmap(data)
			return nil, errors.New("unsupported sip message file version")
		}
		count = (size - 4) / (8 + msgSize)
	} else {
		version = 1
		if size%msgSize != 0 {
			syscall.Munmap(data)
			return nil, errors.New("invalid sip message file size")
		}
		count = size / msgSize
	}
	return &File{
		path:    path,
		data:    data,
		version: version,
		count:   count,
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

// Count returns the number of messages in the file.
func (r *File) Count() int {
	return r.count
}

// Get returns a pointer to the message at the given index.
func (r *File) Get(index int) (msg *Message, received clocky.Time) {
	switch r.version {
	case 1:
		received = 0
		msg = (*Message)(unsafe.Pointer(&r.data[index*msgSize]))
	case 2:
		i := 4 + index*(8+msgSize)
		received = clocky.Time(binary.LittleEndian.Uint64(r.data[i : i+8]))
		msg = (*Message)(unsafe.Pointer(&r.data[i+8]))
	default:
		panic("invalid file version")
	}
	return
}

// EOF returns whether we have reached the end of the file.
func (r *File) EOF() bool {
	return r.pos == r.count
}

// Read returns a pointer to the next message in the file.
// Returns nil when there are no more messages.
func (r *File) Read() (msg *Message, received clocky.Time) {
	if r.pos == r.count {
		return nil, 0
	}
	msg, received = r.Get(r.pos)
	r.pos++
	return msg, received
}

// Peek returns a pointer to the next message without advancing the position.
// Returns nil when there are no more messages.
func (r *File) Peek() (msg *Message, received clocky.Time) {
	if r.pos == r.count {
		return nil, 0
	}
	msg, received = r.Get(r.pos)
	return msg, received
}

const msgSize = int(unsafe.Sizeof(Message{}))
