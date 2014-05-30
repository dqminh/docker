package main

import (
	"flag"
	"log"
	"net"

	"github.com/dotcloud/docker/vfuse/client"
)

var (
	root = flag.String("root", ".", "Directory to share.")
	rw   = flag.Bool("writable", true, "whether -root is writable")
	addr = flag.String("addr", "localhost:4321", "dockerfs service address")
)

func main() {
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		log.Panic(err)
	}

	srv := client.NewServer(conn, *root, *rw)
	srv.Run()
}
