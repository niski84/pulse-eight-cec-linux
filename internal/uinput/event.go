package uinput

import (
	"encoding/binary"
	"io"
)

type inputEvent struct {
	seconds      int64
	microseconds int64
	eventType    uint16
	code         uint16
	value        int32
}

func writeInputEvent(writer io.Writer, event inputEvent) error {
	var data [24]byte
	binary.LittleEndian.PutUint64(data[0:8], uint64(event.seconds))
	binary.LittleEndian.PutUint64(data[8:16], uint64(event.microseconds))
	binary.LittleEndian.PutUint16(data[16:18], event.eventType)
	binary.LittleEndian.PutUint16(data[18:20], event.code)
	binary.LittleEndian.PutUint32(data[20:24], uint32(event.value))
	_, err := io.CopyN(writer, &byteReader{data: data[:]}, int64(len(data)))
	return err
}

type byteReader struct {
	data []byte
}

func (r *byteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
