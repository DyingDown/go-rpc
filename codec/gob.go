package codec

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"io"

	"github.com/sirupsen/logrus"
)

type GobCodec struct {
	conn io.ReadWriteCloser
	buff *bufio.ReadWriter
	dec  *gob.Decoder
	enc  *gob.Encoder
}

var _ Codec = (*GobCodec)(nil)

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	readbuf := bufio.NewReader(conn)
	writebuf := bufio.NewWriter(conn)
	buf := bufio.NewReadWriter(readbuf, writebuf)
	return &GobCodec{
		buff: buf,
		conn: conn,
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(conn),
	}
}

func (codec *GobCodec) ReadHeader(h *Header) error {
	var length uint32
	binary.Read(codec.buff, binary.BigEndian, &length)
	raw := make([]byte, length)
	codec.buff.Read(raw)
	buf := bytes.NewBuffer(raw)
	return gob.NewDecoder(buf).Decode(h)
}

func (codec *GobCodec) ReadBody(body interface{}) error {
	var length uint32
	binary.Read(codec.buff, binary.BigEndian, &length)
	raw := make([]byte, length)
	codec.buff.Read(raw)
	buf := bytes.NewBuffer(raw)
	return gob.NewDecoder(buf).Decode(body)
}

func (codec *GobCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		codec.buff.Flush()
		if err != nil {
			codec.Close()
		}
	}()
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(h); err != nil {
		logrus.Errorf("fail to encode header: %v", err)
		return err
	}
	binary.Write(codec.buff, binary.BigEndian, uint32(buf.Len()))
	codec.buff.Write(buf.Bytes())
	buf.Reset()
	if err := gob.NewEncoder(buf).Encode(body); err != nil {
		logrus.Errorf("fail to encode body: %v", err)
		return err
	}
	binary.Write(codec.buff, binary.BigEndian, uint32(buf.Len()))
	codec.buff.Write(buf.Bytes())
	return nil
}

func (codec *GobCodec) Close() error {
	return codec.conn.Close()
}
