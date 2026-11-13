package wallets

import "encoding/base64"

type xdrWriter struct {
	buf []byte
}

func (w *xdrWriter) uint32(value uint32) {
	w.buf = append(w.buf, byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
}

func (w *xdrWriter) int32(value int32) {
	w.uint32(uint32(value))
}

func (w *xdrWriter) uint64(value uint64) {
	w.buf = append(w.buf,
		byte(value>>56), byte(value>>48), byte(value>>40), byte(value>>32),
		byte(value>>24), byte(value>>16), byte(value>>8), byte(value),
	)
}

func (w *xdrWriter) int64(value int64) {
	w.uint64(uint64(value))
}

func (w *xdrWriter) bool(value bool) {
	if value {
		w.int32(1)
		return
	}
	w.int32(0)
}

func (w *xdrWriter) fixedOpaque(value []byte) {
	w.buf = append(w.buf, value...)
	for len(w.buf)%4 != 0 {
		w.buf = append(w.buf, 0)
	}
}

func (w *xdrWriter) varOpaque(value []byte) {
	w.uint32(uint32(len(value)))
	w.fixedOpaque(value)
}

func (w *xdrWriter) base64() string {
	return base64.StdEncoding.EncodeToString(w.buf)
}
