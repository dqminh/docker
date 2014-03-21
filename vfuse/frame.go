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
	AttrReqType: parseAttrReqPacket,
	AttrResType: parseAttrResPacket,
}

func parseAttrReqPacket(h Header, body []byte) (Packet, error) {
	return AttrReqPacket{
		Hdr:  h,
		Name: string(body),
	}, nil
}

func parseAttrResPacket(h Header, body []byte) (Packet, error) {
	if len(body) != AttrResLength {
		return nil, errors.New("unexpected attr res length")
	}
	p := AttrResPacket{Hdr: h}
	p.Attr.Size, body = eatu64(body)
	p.Attr.Atime, body = eatu64(body)
	p.Attr.Mtime, body = eatu64(body)
	p.Attr.Atimensec, body = eatu32(body)
	p.Attr.Mtimensec, body = eatu32(body)
	p.Attr.Mode, body = eatu32(body)
	p.Attr.Nlink, body = eatu32(body)
	return p, nil

}

func eatu64(b []byte) (uint64, []byte) {
	return binary.BigEndian.Uint64(b[:8]), b[8:]
}

func eatu32(b []byte) (uint32, []byte) {
	return binary.BigEndian.Uint32(b[:4]), b[4:]
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
	Size      uint64
	Atime     uint64
	Mtime     uint64
	Atimensec uint32
	Mtimensec uint32
	Mode      uint32
	Nlink     uint32
}

const AttrResLength = 8*3 + 4*4

func NewAttrResPacket(id uint64, a Attr) AttrResPacket {
	return AttrResPacket{
		Hdr: Header{
			ID:     id,
			Type:   AttrResType,
			Length: uint32(AttrResLength),
		},
		Attr: a,
	}
}

func (p AttrResPacket) Header() Header {
	return p.Hdr
}

func (p AttrResPacket) RawBody() []byte {
	buf := make([]byte, 0, p.Hdr.Length)
	buf = appendu64(buf, p.Attr.Size)
	buf = appendu64(buf, p.Attr.Atime)
	buf = appendu64(buf, p.Attr.Mtime)
	buf = appendu32(buf, p.Attr.Atimensec)
	buf = appendu32(buf, p.Attr.Mtimensec)
	buf = appendu32(buf, p.Attr.Mode)
	buf = appendu32(buf, p.Attr.Nlink)
	return buf
}

func appendu64(src []byte, v uint64) []byte {
	binary.BigEndian.PutUint64(src[len(src):len(src)+8], v)
	return src[:len(src)+8]
}

func appendu32(src []byte, v uint32) []byte {
	binary.BigEndian.PutUint32(src[len(src):len(src)+4], v)
	return src[:len(src)+4]
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
