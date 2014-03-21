package vfuse

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

type PacketType uint32

const (
	AttrReqType PacketType = 0
	AttrResType PacketType = 1
)

// Registry of packet parsing functions. The body
var parsers = map[PacketType]func(h Header, body []byte) (Packet, error){
	AttrReqType: parseAddrReqPacket,
}

func parseAddrReqPacket(h Header, body []byte) (Packet, error) {
	return AttrReqPacket{
		Hdr:  h,
		Name: string(body),
	}, nil
}

type Header struct {
	ID     uint64
	Type   PacketType
	Length uint32
}

type Packet interface {
	Header() Header
	RawBody() []byte
}

type OpenPacket struct {
}

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
		Type:   PacketType(binary.BigEndian.Uint32(hbuf[8:12])),
		Length: binary.BigEndian.Uint32(hbuf[12:]),
	}

	if h.Length > MaxLength {
		return nil, errors.New("Length too long")
	}

	body := make([]byte, h.Length)
	_, err = io.ReadFull(c.buf, body)
	if err != nil {
		return nil, err
	}
	ctor, ok := parsers[h.Type]
	if !ok {
		return nil, fmt.Errorf("Unknown packet type %d received", h.Type)
	}
	return ctor(h, body)
}

func (c *Client) WritePacket(p Packet) error {
	h, body := p.Header(), p.RawBody()
	var hbuf [16]byte // TODO(philips): garbage

	if int(h.Length) != len(body) {
		panic("Body length mismatch")
	}

	binary.BigEndian.PutUint64(hbuf[:8], h.ID)
	binary.BigEndian.PutUint32(hbuf[8:12], uint32(h.Type))
	binary.BigEndian.PutUint32(hbuf[12:], h.Length)

	if _, err := c.buf.Write(hbuf[:]); err != nil {
		return err
	}
	if _, err := c.buf.Write(body); err != nil {
		return err
	}

	return c.buf.Flush()
}

type AttrResPacket struct {
	Hdr  Header
	Attr Attr
}

type Attr struct {
	Size uint64
	Atime uint64
	Mtime uint64
	Atimensec uint32
	Mtimensec uint32
	Mode uint32
	Nlink uint32
}

func NewAttrResPacket(id uint64, name string) AttrResPacket {
	return AttrResPacket{
		Header{
			ID:     id,
			Type:   AttrResType,
			Length: uint32(len(Attr)),
		},
		name,
	}
}

func (p AttrResPacket) Header() Header {
	return p.Hdr
}

func (p AttrResPacket) RawBody() []byte {
	return []byte(p.Name)
}

type AttrReqPacket struct {
	Hdr  Header
	Name string
}

func NewAttrReqPacket(id uint64, name string) AttrReqPacket {
	return AttrReqPacket{
		Header{
			ID:     id,
			Type:   AttrReqType,
			Length: uint32(len(name)),
		},
		name,
	}
}

func (p AttrReqPacket) Header() Header {
	return p.Hdr
}

func (p AttrReqPacket) RawBody() []byte {
	return []byte(p.Name)
}
