// vfused is the vfuse daemon: it forward to its client all the
// filesystem operations done on a given mount point.
package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"

	"github.com/docker/docker/pkg/vfuse/server"
)

var (
	listenAddr = flag.String("listen", "7070", "Listen port or 'ip:port'.")
	mount      = flag.String("mount", "", "Mount point. If empty, a temp directory is used.")
	verbose    = flag.Bool("verbose", false, "verbose debugging mode")
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
	server.Verbose = *verbose

	connc := make(chan net.Conn, 1)
	ln, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("Listen: %v", err)
	}
	go func() {
		defer ln.Close()
		c, err := ln.Accept()
		if err != nil {
			log.Fatalf("Error accepting conn: %v", err)
		}
		connc <- c
	}()

	srv, err := server.NewServer(*mount, func() net.Conn {
		return <-connc
	})
	if err != nil {
		log.Fatalf("error creating server: %v", err)
	}

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
