package codec

import (
	"bufio"
	"encoding/gob"
	"io"

	"github.com/sirupsen/logrus"
)

type GobCodec struct {
	conn io.ReadWriteCloser
	buff bufio.Writer
	dec  *gob.Decoder
	enc  *gob.Encoder
}

var _ Codec = (*GobCodec)(nil)

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buff: *buf,
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(buf),
	}
}

func (codec *GobCodec) ReadHeader(h *Header) error {
	logrus.Info("read header456")
	err := codec.dec.Decode(h)
	logrus.Info("decode done")
	return err
}

func (codec *GobCodec) ReadBody(body interface{}) error {
	return codec.dec.Decode(body)
}

func (codec *GobCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		codec.buff.Flush()
		if err != nil {
			codec.Close()
		}
	}()
	if err := codec.enc.Encode(h); err != nil {
		logrus.Errorf("fail to encode header: %v", err)
		return err
	}
	if err := codec.enc.Encode(body); err != nil {
		logrus.Errorf("fail to encode body: %v", err)
		return err
	}
	return nil
}

func (codec *GobCodec) Close() error {
	return codec.conn.Close()
}
