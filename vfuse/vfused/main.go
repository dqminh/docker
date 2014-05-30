package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"strconv"

	"github.com/dotcloud/docker/vfuse/server"
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

	srv, err := server.NewServer(*listenAddr, *mount)
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
