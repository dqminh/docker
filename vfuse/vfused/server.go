package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/dotcloud/docker/vfuse"
	"github.com/hanwen/go-fuse/fuse"
)

var (
	listenAddr = flag.String("listen", "7070", "Listen port or 'ip:port'.")
	mount      = flag.String("mount", "", "Mount point. If empty, a temp directory is used.")
)

func main() {
	flag.Parse()
	if flag.NArg() > 0 {
		log.Fatalf("No args supported.")
	}
	if *mount == "" {
		var err error
		*mount, err = ioutil.TempDir("", "vfused-tmp")
		if err != nil {
			log.Fatal(err)
		}
		defer os.Remove(*mount)
	}
	if _, err := strconv.Atoi(*listenAddr); err == nil {
		*listenAddr = ":" + *listenAddr
	}
	ln, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("Listen: %v", err)
	}
	opts := &fuse.MountOptions{
		Name: "vfuse_SOMECLIENT",
	}
	fs := NewFS(ln)
	rawFS := fuse.NewRawFileSystem(fs)
	log.Printf("Mounting at %s", *mount)
	srv, err := fuse.NewServer(rawFS, *mount, opts)
	if err != nil {
		log.Fatalf("NewServer: %v", err)
	}
	go srv.Serve()

	go func() {
		c, err := net.Dial("tcp", *listenAddr)
		if err != nil {
			log.Printf("Client dial fail: %v", err)
			return
		}
		c.Close()
	}()

	log.Printf("Press 'q'+<enter> to exit.")
	var buf [1]byte
	for {
		_, err := os.Stdin.Read(buf[:])
		if err != nil || buf[0] == 'q' {
			break
		}
	}
	log.Printf("Got key, unmounting.")
	srv.Unmount()
	log.Printf("Unmounted, quitting.")
}

type FS struct {
	ln net.Listener
	c  net.Conn
	vc *vfuse.Client

	mu     sync.Mutex // guards writing to vc and following fields
	nextid uint64
	res    map[uint64]chan<- vfuse.Packet
}

func NewFS(ln net.Listener) *FS {
	return &FS{
		ln:  ln,
		res: make(map[uint64]chan<- vfuse.Packet),
	}
}

func (fs *FS) Init(s *fuse.Server) {
	log.Printf("fs.Init. Waiting for conn from %v", fs.ln.Addr())
	c, err := fs.ln.Accept()
	if err != nil {
		log.Printf("Error accepting conn: %v", err)
		s.Unmount()
		return
	}
	fs.ln.Close()
	fs.c = c
	fs.vc = vfuse.NewClient(c)
	go fs.readFromClient()
	log.Printf("Init got conn %v from %v", c, c.RemoteAddr())
}

func (fs *FS) StatFs(in *fuse.InHeader, out *fuse.StatfsOut) (code fuse.Status) {
	log.Printf("fs.StatFs")
	// TODO(bradfitz): make up some stuff for now. Do this properly later
	// with a new packet type to the client.
	out.Bsize = 1024
	out.Blocks = 1e6
	out.Bfree = out.Blocks / 2
	out.Bavail = out.Blocks / 2
	out.Files = 1e3
	out.Ffree = 1e3 - 2
	return 0
}

func (fs *FS) readFromClient() {
	for {
		p, err := fs.vc.ReadPacket()
		if err != nil {
			log.Fatalf("Client disconnected or something: %v", err)
		}
		fs.mu.Lock()
		id := p.Header().ID
		resc, ok := fs.res[id]
		if ok {
			delete(fs.res, id)
		}
		fs.mu.Unlock()
		if !ok {
			log.Fatalf("Client sent bogus packet we didn't ask for")
		}
		resc <- p
	}
}
