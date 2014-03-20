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
	fs := &FS{
		ln: ln,
	}
	rawFS := fuse.NewRawFileSystem(fs)
	log.Printf("Mounting at %s", *mount)
	srv, err := fuse.NewServer(rawFS, *mount, opts)
	if err != nil {
		log.Fatalf("NewServer: %v", err)
	}
	go srv.Serve()
	log.Printf("Waiting for key to exit.")
	os.Stdin.Read(make([]byte, 1))
	log.Printf("Got key, unmounting.")
	srv.Unmount()
	log.Printf("Unmounted, quitting.")
}

type FS struct {
	ln net.Listener
	c  net.Conn
	vc *vfuse.Client

	wmu sync.Mutex // mutex to hold while writing a packet
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
	log.Printf("Init got conn %v from %v", c, c.RemoteAddr())
}
