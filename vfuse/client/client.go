// A Go mirror of libfuse's hello.c
package main

import (
	"flag"
	"path/filepath"
	"log"
	"fmt"
	"sync"
	"os"
	"net"

	"github.com/dotcloud/docker/vfuse"
)

type Volume struct {
	Root string

	mu sync.Mutex
	files map[uint64]*os.File
}

func NewVolume(root string) *Volume {
	return &Volume{
		Root: root,
		files: make(map[uint64]*os.File),
	}

}

var addr = flag.String("addr", "localhost:4321", "dockerfs service address")

func main() {
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		log.Panic(err)
	}

	v := NewVolume(".")

	c := vfuse.NewClient(conn)


	for {
		pkti, err := c.ReadPacket()
		if err != nil {
			log.Fatal("ReadPacket error: %v", err)
		}

		log.Printf("Got packet %T %+v %q", pkti, pkti.Header(), pkti.RawBody())

		switch pkt := pkti.(type) {
		case vfuse.AttrReqPacket:
			fi, _ := os.Lstat(filepath.Join(v.Root, filepath.FromSlash(pkt.Name)))
			c.WritePacket(Packet{
		}

	}
}
