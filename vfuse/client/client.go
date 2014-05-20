// The client binary owns the filesystem and runs on the host machine
// (which may be Mac, Windows, Linux, etc) and responds to the dockerd
// server (which is running FUSE, and always Linux).
package main

import (
	"flag"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/dotcloud/docker/vfuse"
	"github.com/dotcloud/docker/vfuse/pb"

	"code.google.com/p/goprotobuf/proto"
)

type Volume struct {
	Root string

	mu    sync.Mutex
	files map[uint64]*os.File
}

func NewVolume(root string) *Volume {
	return &Volume{
		Root:  root,
		files: make(map[uint64]*os.File),
	}

}

var addr = flag.String("addr", "localhost:4321", "dockerfs service address")

var _ = proto.String

func main() {
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		log.Panic(err)
	}

	v := NewVolume(".")

	c := vfuse.NewClient(conn)

	for {
		p, err := c.ReadPacket()
		if err != nil {
			log.Fatal("client: ReadPacket error: %v", err)
		}

		log.Printf("client: got packet %+v %T", p.Header, p.Body)

		switch m := p.Body.(type) {
		case *pb.AttrRequest:
			fi, err := os.Lstat(filepath.Join(v.Root, filepath.FromSlash(m.GetName())))
			res := new(pb.AttrResponse)
			if err != nil {
				if os.IsNotExist(err) {
					res.Err = &pb.Error{NotExist: proto.Bool(true)}
				} else {
					// TODO: more specific types
					res.Err = &pb.Error{Other: proto.String(err.Error())}
				}
			} else {
				res.Attr = &pb.Attr{
					Size: proto.Uint64(uint64(fi.Size())),
					// TODO: more
				}
			}
			err = c.WriteReply(p.ID, res)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}
