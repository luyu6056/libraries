package libraries

import (
	"io"
	"unsafe"
)

type MsgBuffer struct {
	b []byte
	l int //长度
	i int //起点位置
}

const (
	blocksize = 1024 * 1024
)

func (w *MsgBuffer) Reset() {
	w.l = 0
	w.i = 0
}
func (w *MsgBuffer) SetBytes(b []byte) {
	w.b = b
	w.i = 0
	w.l = len(b)
}
func (w *MsgBuffer) Make(l int) []byte {
	if w.i > blocksize {
		copy(w.b[:w.l-w.i], w.b[w.i:w.l])
		w.l -= w.i
		w.i = 0
	}
	o := w.l
	w.l += l
	if len(w.b) < w.l { //扩容
		if l > blocksize {
			w.b = append(w.b, make([]byte, l)...)
		} else {
			w.b = append(w.b, make([]byte, blocksize)...)
		}
	}
	return w.b[o:w.l]
}
func (w *MsgBuffer) Write(b []byte) (int, error) {
	if w.i > blocksize {
		copy(w.b[:w.l-w.i], w.b[w.i:w.l])
		w.l -= w.i
		w.i = 0
	}
	l := len(b)
	o := w.l
	w.l += l
	if len(w.b) < w.l {
		if l > blocksize {
			w.b = append(w.b, make([]byte, l)...)
		} else {
			w.b = append(w.b, make([]byte, blocksize)...)
		}

	}
	copy(w.b[o:w.l], b)
	return l, nil
}
func (w *MsgBuffer) WriteString(s string) {
	if w.i > blocksize {
		copy(w.b[:w.l-w.i], w.b[w.i:w.l])
		w.l -= w.i
		w.i = 0
	}
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	b := *(*[]byte)(unsafe.Pointer(&h))
	l := len(b)
	o := w.l
	w.l += l
	if len(w.b) < w.l {
		if l > blocksize {
			w.b = append(w.b, make([]byte, l)...)
		} else {
			w.b = append(w.b, make([]byte, blocksize)...)
		}
	}
	copy(w.b[o:w.l], b)
}
func (w *MsgBuffer) WriteByte(s byte) {
	if w.i > blocksize {
		copy(w.b[:w.l-w.i], w.b[w.i:w.l])
		w.l -= w.i
		w.i = 0
	}
	w.l++
	if len(w.b) < w.l {
		w.b = append(w.b, make([]byte, blocksize)...)
	}
	w.b[w.l-1] = s
}
func (w *MsgBuffer) Bytes() []byte {
	return w.b[w.i:w.l]
}

func (w *MsgBuffer) Len() int {
	return w.l - w.i
}
func (w *MsgBuffer) Next(l int) []byte {
	o := w.i
	w.i += l
	if w.i > w.l {
		w.i = w.l
	}
	return w.b[o:w.i]
}
func (w *MsgBuffer) Truncate(i int) {
	w.l = w.i + i
}
func (w *MsgBuffer) String() string {
	b := make([]byte, w.l-w.i)
	copy(b, w.b[w.i:w.l])
	return *(*string)(unsafe.Pointer(&b))
}

// New returns a new MsgBuffer whose buffer has the given size.
func New(size int) *MsgBuffer {

	return &MsgBuffer{
		b: make([]byte, size),
	}
}

// Shift shifts the "read" pointer.
func (r *MsgBuffer) Shift(len int) {
	if len <= 0 {
		return
	}
	if len < r.Length() {
		r.i += len
		if r.i > r.l {
			r.i = r.l
		}
	} else {
		r.Reset()
	}
}

func (r *MsgBuffer) Close() error {
	return nil
}

// Read reads up to len(p) bytes into p. It returns the number of bytes read (0 <= n <= len(p)) and any error encountered.
// Even if Read returns n < len(p), it may use all of p as scratch space during the call.
// If some data is available but not len(p) bytes, Read conventionally returns what is available instead of waiting for more.
// When Read encounters an error or end-of-file condition after successfully reading n > 0 bytes,
// it returns the number of bytes read. It may return the (non-nil) error from the same call or return the error (and n == 0) from a subsequent call.
// Callers should always process the n > 0 bytes returned before considering the error err.
// Doing so correctly handles I/O errors that happen after reading some bytes and also both of the allowed EOF behaviors.
func (r *MsgBuffer) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.i == r.l {
		return 0, io.EOF
	}
	o := r.i
	r.i += len(p)
	if r.i > r.l {
		r.i = r.l
	}
	copy(p, r.b[o:r.i])
	return r.i - o, nil
}

// ReadByte reads and returns the next byte from the input or ErrIsEmpty.
func (r *MsgBuffer) ReadByte() (b byte, err error) {
	if r.i == r.l {
		return 0, io.EOF
	}
	b = r.b[r.i]
	r.i++
	return b, err
}
func (r *MsgBuffer) IsEmpty() bool {
	return r.l == r.i
}

// Write writes len(p) bytes from p to the underlying buf.
// It returns the number of bytes written from p (n == len(p) > 0) and any error encountered that caused the write to stop early.
// If the length of p is greater than the writable capacity of this ring-buffer, it will allocate more memory to this ring-buffer.
// Write must not modify the slice data, even temporarily.

// Length return the length of available read bytes.
func (r *MsgBuffer) Length() int {
	return r.l - r.i
}

// Capacity returns the size of the underlying buffer.
func (r *MsgBuffer) Capacity() int {
	return len(r.b)
}

// Free returns the length of available bytes to write.
/*func (r *MsgBuffer) Free() int {
	if r.r == r.w {
		if r.isEmpty {
			return r.size
		}
		return 0
	}

	if r.w < r.r {
		return r.r - r.w
	}

	return r.size - r.w + r.r
}



// WithBytes combines the available read bytes and the given bytes. It does not move the read pointer and only copy the available data.
func (r *MsgBuffer) WithBytes(b []byte) []byte {
	bn := len(b)
	if r.isEmpty {
		return b
	} else if r.r == r.w {
		buf := pbytes.GetLen(r.size + bn)
		copy(buf, r.buf)
		copy(buf[r.size:], b)
		return buf
	}

	if r.w > r.r {
		buf := pbytes.GetLen(r.w - r.r + bn)
		copy(buf, r.buf[r.r:r.w])
		copy(buf[r.w-r.r:], b)
		return buf
	}

	n := r.size - r.r + r.w
	buf := pbytes.GetLen(n + bn)

	if r.r+n < r.size {
		copy(buf, r.buf[r.r:r.r+n])
	} else {
		c1 := r.size - r.r
		copy(buf, r.buf[r.r:r.size])
		c2 := n - c1
		copy(buf[c1:], r.buf[0:c2])
	}
	copy(buf[n:], b)

	return buf
}

// Recycle recycles slice of bytes.
func Recycle(p []byte) {
	pbytes.Put(p)
}

// IsFull returns this MsgBuffer is full.
func (r *MsgBuffer) IsFull() bool {
	return r.r == r.w && !r.isEmpty
}

// IsEmpty returns this MsgBuffer is empty.
func (r *MsgBuffer) IsEmpty() bool {
	return r.isEmpty
}

// Reset the read pointer and writer pointer to zero.
func (r *MsgBuffer) Reset() {
	r.r = 0
	r.w = 0
	r.isEmpty = true
}

func (r *MsgBuffer) malloc(cap int) {
	newCap := internal.CeilToPowerOfTwo(r.size + cap)
	//newBuf := pbytes.GetLen(newCap)
	newBuf := make([]byte, newCap)
	oldLen := r.Length()
	_, _ = r.Read(newBuf)
	r.r = 0
	r.w = oldLen
	r.size = newCap
	r.mask = newCap - 1
	r.buf = newBuf
}*/
func (r *MsgBuffer) WithBytes(b []byte) []byte {
	r.Write(b)
	return r.Next(r.Len())
}
func (r *MsgBuffer) LazyRead(n int) ([]byte, []byte) {
	if n > r.Len() {
		n = r.Len()
	}
	return r.Bytes()[:n], nil
}
func (r *MsgBuffer) LazyReadAll() ([]byte, []byte) {
	return r.Bytes(), nil
}
