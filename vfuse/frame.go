package vfuse

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"sync"

	"code.google.com/p/goprotobuf/proto"
	"github.com/dotcloud/docker/vfuse/pb"
)

type PacketType uint32

var (
	maxPacketType   PacketType
	messageFromType = map[PacketType]func() proto.Message{}
)

func newFunc(m proto.Message) func() proto.Message {
	t := reflect.TypeOf(m).Elem()
	return func() proto.Message {
		return reflect.New(t).Interface().(proto.Message)
	}
}

func addRPC(req, res proto.Message) {
	if req != nil {
		messageFromType[maxPacketType+0] = newFunc(req)
	}
	if res != nil {
		messageFromType[maxPacketType+1] = newFunc(res)
	}
	maxPacketType += 2
}

func init() {
	// Warning: order matters here. Don't re-order, insert, or delete
	// items.
	//
	// Only append things to the end. If you have to delete, replace
	// with nil, nil instead.
	addRPC(&pb.AttrRequest{}, &pb.AttrResponse{})
	addRPC(&pb.ReaddirRequest{}, &pb.ReaddirResponse{})
	addRPC(&pb.ReadlinkRequest{}, &pb.ReadlinkResponse{})
	addRPC(&pb.OpenRequest{}, &pb.OpenResponse{})
	addRPC(&pb.CreateRequest{}, &pb.CreateResponse{})
	addRPC(&pb.ReadRequest{}, &pb.ReadResponse{})
	addRPC(&pb.WriteRequest{}, &pb.WriteResponse{})
	addRPC(&pb.ChmodRequest{}, &pb.ChmodResponse{})
	addRPC(&pb.ChownRequest{}, &pb.ChownResponse{})
	addRPC(&pb.TruncateRequest{}, &pb.TruncateResponse{})
	addRPC(&pb.UtimeRequest{}, &pb.UtimeResponse{})
	addRPC(&pb.LinkRequest{}, &pb.LinkResponse{})
	addRPC(&pb.SymlinkRequest{}, &pb.SymlinkResponse{})
	addRPC(&pb.MkdirRequest{}, &pb.MkdirResponse{})
	addRPC(&pb.RenameRequest{}, &pb.RenameResponse{})
	addRPC(&pb.RmdirRequest{}, &pb.RmdirResponse{})
	addRPC(&pb.UnlinkRequest{}, &pb.UnlinkResponse{})
	addRPC(&pb.MknodRequest{}, &pb.MknodResponse{})
	addRPC(&pb.CloseRequest{}, &pb.CloseResponse{})
}

var packetTypeFromMessage = map[reflect.Type]PacketType{}

func init() {
	for pt, newMsg := range messageFromType {
		rt := reflect.TypeOf(newMsg())
		if _, dup := packetTypeFromMessage[rt]; dup {
			panic(fmt.Sprintf("Duplicate registration in messageFromType of type %v", rt))
		}
		packetTypeFromMessage[rt] = pt
	}
}

const (
	// MaxLength is the maximum length of a packet's payload.
	// It does not include the size of the header.
	MaxLength = 16 << 20
)

type Header struct {
	// ID is unique per-request and echoed back in responses.
	ID     uint64
	Type   PacketType
	Length uint32
}

type Packet struct {
	Header
	Body proto.Message
}

type Client struct {
	buf  *bufio.ReadWriter
	conn net.Conn

	wmu      sync.Mutex // guards writing to buf and wscratch
	wscratch [16]byte
}

func NewClient(conn net.Conn) *Client {
	return &Client{
		conn: conn,
		buf:  bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn)),
	}
}

func (c *Client) ReadPacket() (p Packet, err error) {
	var hbuf [16]byte // TODO(philips): garbage

	_, err = io.ReadFull(c.buf, hbuf[:])
	if err != nil {
		return Packet{}, err
	}

	p.Header = Header{
		ID:     binary.BigEndian.Uint64(hbuf[:8]),
		Type:   PacketType(binary.BigEndian.Uint32(hbuf[8:12])),
		Length: binary.BigEndian.Uint32(hbuf[12:]),
	}

	if p.Header.Length > MaxLength {
		return Packet{}, errors.New("Length too long")
	}

	body := make([]byte, p.Header.Length)
	_, err = io.ReadFull(c.buf, body)
	if err != nil {
		return
	}
	newMsg, ok := messageFromType[p.Header.Type]
	if !ok {
		return Packet{}, fmt.Errorf("Unknown packet type %d received", p.Header.Type)
	}
	p.Body = newMsg()
	err = proto.Unmarshal(body, p.Body)
	return
}

// WritePacket writes p to client. Only the ID and Body need to be set.
// The Type and Length are automatic.
func (c *Client) WritePacket(p Packet) error {
	pt, ok := packetTypeFromMessage[reflect.TypeOf(p.Body)]
	if !ok {
		panic(fmt.Sprintf("unregistered body message type %T", p.Body))
	}
	body, err := proto.Marshal(p.Body)
	if err != nil {
		return err
	}
	if len(body) > MaxLength {
		return fmt.Errorf("proto-encoded message %T of length %d exceeds maximum size of %d",
			p.Body, len(body), MaxLength)
	}

	c.wmu.Lock()
	defer c.wmu.Unlock()

	hbuf := c.wscratch[:]
	binary.BigEndian.PutUint64(hbuf[:8], p.ID)
	binary.BigEndian.PutUint32(hbuf[8:12], uint32(pt))
	binary.BigEndian.PutUint32(hbuf[12:], uint32(len(body)))

	if _, err := c.buf.Write(hbuf); err != nil {
		return err
	}
	if _, err := c.buf.Write(body); err != nil {
		return err
	}
	return c.buf.Flush()
}

func (c *Client) WriteReply(id uint64, body proto.Message) error {
	return c.WritePacket(Packet{
		Header: Header{ID: id},
		Body:   body,
	})
}
