package vfuse

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
)

const (
	ReadFile = 0
	ReadDir  = 1
)

type Header struct {
	ID     uint64
	Type   uint32
	Length uint32
}

type Packet interface {
	Header() Header
	RawBody() []byte
}

type rawPacket struct {
	h Header
	b []byte
}

func (rp rawPacket) Header() Header  { return rp.h }
func (rp rawPacket) RawBody() []byte { return rp.b }

type Client struct {
	buf  *bufio.ReadWriter
	conn net.Conn
}

const (
	MaxLength = 16 << 20
)

func NewClient(conn net.Conn) *Client {
	return &Client{
		conn: conn,
		buf:  bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn)),
	}
}

func (c *Client) ReadPacket() (Packet, error) {
	var hbuf [16]byte // TODO(philips): garbage

	_, err := io.ReadFull(c.buf, hbuf[:])
	if err != nil {
		return nil, err
	}

	h := Header{
		ID:     binary.BigEndian.Uint64(hbuf[:8]),
		Type:   binary.BigEndian.Uint32(hbuf[8:12]),
		Length: binary.BigEndian.Uint32(hbuf[12:]),
	}

	if h.Length > MaxLength {
		return nil, errors.New("Length too long")
	}

	body := make([]byte, h.Length)
	_, err = io.ReadFull(c.buf, body)

	return rawPacket{h, body}, err
}

func (c *Client) WritePacket(p Packet) error {
	h, body := p.Header(), p.RawBody()
	var hbuf [16]byte // TODO(philips): garbage

	if int(h.Length) != len(body) {
		panic("Body length mismatch")
	}

	binary.BigEndian.PutUint64(hbuf[:8], h.ID)
	binary.BigEndian.PutUint32(hbuf[8:12], h.Type)
	binary.BigEndian.PutUint32(hbuf[12:], h.Length)

	if _, err := c.buf.Write(hbuf[:]); err != nil {
		return err
	}
	if _, err := c.buf.Write(body); err != nil {
		return err
	}

	return c.buf.Flush()
}
