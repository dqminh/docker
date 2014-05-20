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

var (
	root    = flag.String("root", ".", "Directory to share.")
	rw      = flag.Bool("writable", true, "whether -root is writable")
	addr    = flag.String("addr", "localhost:4321", "dockerfs service address")
	verbose = flag.Bool("verbose", false, "verbose debugging mode")
)

type Volume struct {
	Root     string
	Writable bool

	mu    sync.Mutex
	files map[uint64]*os.File
}

// Server is the server that runs on the client (filesystem host)
// server.  It accepts FUSE proxy requests from the peer (the Linux
// dockerd running a real FUSE filesystem) and responds with the FUSE
// answers from vol.
type Server struct {
	// TODO(bradfitz): perhaps rename some things here to avoid both
	// the words "server" and "client".
	vol *Volume
	c   *vfuse.Client
}

func NewVolume(root string, writable bool) *Volume {
	return &Volume{
		Root:     root,
		Writable: writable,

		files: make(map[uint64]*os.File),
	}

}

func vlogf(format string, args ...interface{}) {
	if !*verbose {
		return
	}
	log.Printf("server: "+format, args...)
}

func main() {
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		log.Panic(err)
	}

	srv := &Server{
		vol: NewVolume(".", *rw),
		c:   vfuse.NewClient(conn),
	}
	srv.Run()
}

func (s *Server) Run() {
	for {
		vlogf("client: reading packet...")
		p, err := s.c.ReadPacket()
		vlogf("client: read packet %v, %v", p, err)
		if err != nil {
			log.Fatal("client: ReadPacket error: %v", err)
		}

		vlogf("client: got packet %+v %T", p.Header, p.Body)

		switch m := p.Body.(type) {
		case *pb.AttrRequest:
			s.handleAttrRequest(p.ID, m)
		default:
			log.Fatalf("client: unhandled request type %T", p.Body)
		}
	}
}

func (s *Server) handleAttrRequest(id uint64, req *pb.AttrRequest) {
	fi, err := os.Lstat(filepath.Join(s.vol.Root, filepath.FromSlash(req.GetName())))
	res := new(pb.AttrResponse)
	if err != nil {
		if os.IsNotExist(err) {
			res.Err = &pb.Error{NotExist: proto.Bool(true)}
		} else {
			// TODO: more specific types
			res.Err = &pb.Error{Other: proto.String(err.Error())}
		}
	} else {
		mode := uint32(fi.Mode() & 0777)
		const (
			S_IFBLK  = 0x6000
			S_IFCHR  = 0x2000
			S_IFDIR  = 0x4000
			S_IFIFO  = 0x1000
			S_IFLNK  = 0xa000
			S_IFREG  = 0x8000
			S_IFSOCK = 0xc000
		)
		if fi.IsDir() {
			mode |= S_IFDIR
		} else if fi.Mode().IsRegular() {
			mode |= S_IFREG
		}
		res.Attr = &pb.Attr{
			Size: proto.Uint64(uint64(fi.Size())),
			Mode: proto.Uint32(mode),
			// TODO: more
		}
	}

	// TODO: factor this out into Run or elsewhere
	err = s.c.WriteReply(id, res)
	if err != nil {
		log.Fatal(err)
	}
}
