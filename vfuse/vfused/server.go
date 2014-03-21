package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dotcloud/docker/vfuse"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
)

var (
	listenAddr = flag.String("listen", "7070", "Listen port or 'ip:port'.")
	mount      = flag.String("mount", "", "Mount point. If empty, a temp directory is used.")
	self       = flag.Bool("self", false, "connect to self")
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
	if *self {
		go func() {
			to := *listenAddr
			if strings.HasPrefix(to, ":") {
				to = "localhost" + to
			}
			log.Printf("Dialing %q ...", to)
			c, err := net.Dial("tcp", to)
			log.Printf("Client dial = %v, %v", c, err)
			if err != nil {
				log.Printf("Client dial fail: %v", err)
				return
			}
			time.Sleep(60 * time.Minute)
			c.Close()
		}()
	}

	opts := &fuse.MountOptions{
		Name: "vfuse_SOMECLIENT",
	}
	_ = opts
	fs := NewFS(ln)

	nfs := pathfs.NewPathNodeFs(fs, nil)

	log.Printf("Mounting at %s", *mount)
	srv, fsConnector, err := nodefs.MountRoot(*mount, nfs.Root(), nil)
	if err != nil {
		log.Fatalf("NewServer: %v", err)
	}
	_ = fsConnector

	go srv.Serve()

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
	pathfs.FileSystem
	ln net.Listener
	c  net.Conn
	vc *vfuse.Client

	clientOnce sync.Once

	mu     sync.Mutex // guards writing to vc and following fields
	nextid uint64
	res    map[uint64]chan<- vfuse.Packet
}

func (fs *FS) getClient() {
	log.Printf("getClient")
	c, err := fs.ln.Accept()
	if err != nil {
		log.Fatalf("Error accepting conn: %v", err)
		return
	}
	fs.ln.Close()
	fs.c = c
	fs.vc = vfuse.NewClient(c)
	go fs.readFromClient()
	log.Printf("Init got conn %v from %v", c, c.RemoteAddr())
}

// fs.mu must be held.
func (fs *FS) nextID() (uint64, <-chan vfuse.Packet) {
	c := make(chan vfuse.Packet, 1)
	id := fs.nextid
	fs.nextid++
	fs.res[id] = c
	return id, c
}

func NewFS(ln net.Listener) *FS {
	return &FS{
		FileSystem: pathfs.NewDefaultFileSystem(),
		ln:         ln,
		res:        make(map[uint64]chan<- vfuse.Packet),
	}
}

func (fs *FS) StatFs(name string) *fuse.StatfsOut {
	log.Printf("fs.StatFs(%q)", name)
	out := new(fuse.StatfsOut)
	// TODO(bradfitz): make up some stuff for now. Do this properly later
	// with a new packet type to the client.
	out.Bsize = 1024
	out.Blocks = 1e6
	out.Bfree = out.Blocks / 2
	out.Bavail = out.Blocks / 2
	out.Files = 1e3
	out.Ffree = 1e3 - 2
	return out
}

func (fs *FS) Open(name string, flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	log.Printf("fs.Open(%q, flags %d)", name, flags)
	return nil, fuse.ENOSYS
}

func (fs *FS) OpenDir(name string, context *fuse.Context) (stream []fuse.DirEntry, code fuse.Status) {
	log.Printf("OpenDir(%q)", name)
	return nil, fuse.OK
}

func (fs *FS) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	log.Printf("fs.GetAttr(%q)", name)
	fs.clientOnce.Do(fs.getClient)

	fs.mu.Lock()
	id, resc := fs.nextID()
	p := vfuse.NewAttrReqPacket(id, name)
	err := fs.vc.WritePacket(p)
	fs.mu.Unlock()

	if err != nil {
		return nil, fuse.EIO
	}
	res := <-resc
	/*attrResPkt, ok := res.(*fuse.AttrResPacket)
	if !ok {
		return nil, fuse.EIO
	}
	*/
	_ = res

	return nil, fuse.ENOENT
}

//SetAttr(input *SetAttrIn, out *AttrOut) (code Status)

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
