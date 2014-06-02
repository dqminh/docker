package main

import (
	"flag"
	"log"
	"net"

	"github.com/dotcloud/docker/vfuse/client"
)

var (
	root    = flag.String("root", ".", "Directory to share.")
	rw      = flag.Bool("writable", true, "whether -root is writable")
	addr    = flag.String("addr", "localhost:4321", "dockerfs service address")
	verbose = flag.Bool("verbose", false, "verbose debugging mode")
)

func main() {
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		log.Panic(err)
	}
	client.Verbose = *verbose

	srv := client.NewServer(conn, *root, *rw)
	srv.Run()
}
