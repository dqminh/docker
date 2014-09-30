package daemon

import (
	"net"
	"sync"

	"github.com/docker/docker/engine"
	"github.com/docker/docker/pkg/log"
)

var (
	fsRelayConnsMu sync.Mutex
	fsRelayConns   = map[string]*net.TCPConn{}
)

func (daemon *Daemon) ContainerFSRelay(job *engine.Job) engine.Status {
	log.Debugf("FSRelay -> %q", job.Args)
	if len(job.Args) != 2 {
		return job.Errorf("Usage: %s fs_id, conn_handle", job.Name)
	}

	ch := daemon.fsHandlers[job.Args[0]]
	delete(daemon.fsHandlers, job.Args[0])

	if ch == nil {
		return job.Errorf("Unknown or duplicate FS handle")
	}
	conn := TakeRelayConn(job.Args[1])
	if conn == nil {
		return job.Errorf("Unknown or duplicate hijack handle")
	}
	ch <- conn
	log.Debugf("<- FSRelay")
	return engine.StatusOK
}

func (daemon *Daemon) RegisterFSHandle(handle string, ch chan<- net.Conn) {
	log.Debugf("RegisterFSHandle -> handler: %s %v", handle, ch)
	daemon.fsHandlers[handle] = ch
	log.Debugf("<- RegisterFSHandle")
}

func TakeRelayConn(handle string) *net.TCPConn {
	log.Debugf("TakeRelayConn -> handle: %s", handle)
	fsRelayConnsMu.Lock()
	defer fsRelayConnsMu.Unlock()
	c := fsRelayConns[handle]
	delete(fsRelayConns, handle)
	log.Debugf("<- TakeRelayConn %v", c)
	return c
}
