package vfuse

import (
	"io"
	"encoding/binary"
	"bufio"
	"net"
	"errors"
)

const (
	ReadFile = 0
	ReadDir = 1
)

type Header struct {
	ID uint64
	Type uint32
	Length uint32
}

type Client struct {
	buf *bufio.ReadWriter
	conn net.Conn
}

const (
	MaxLength = 16<<20
)

func NewClient(conn net.Conn) *Client {
	return &Client{
		conn: conn,
		buf: bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn)),
	}
}

func (c *Client) ReadPacket() (*Header, []byte, error) {
	var hbuf [16]byte // TODO(philips): garbage

	_, err := io.ReadFull(c.buf, hbuf[:])
	if err != nil {
		return nil, nil, err
	}

	h := &Header{}
	h.ID = binary.BigEndian.Uint64(hbuf[:8])
	h.Type = binary.BigEndian.Uint32(hbuf[8:12])
	h.Length = binary.BigEndian.Uint32(hbuf[12:])

	if h.Length > MaxLength {
		return nil, nil, errors.New("Length too long")
	}

	body := make([]byte, h.Length)
	_, err = io.ReadFull(c.buf, body)

	return h, body, err
}

func (c *Client) WritePacket(h Header, body []byte) error {
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
