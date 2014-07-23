package daemon

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"

	"github.com/docker/docker/engine"
	"github.com/docker/docker/pkg/log"
	"github.com/docker/docker/runconfig"
)

func (daemon *Daemon) ContainerStart(job *engine.Job) engine.Status {
	if len(job.Args) < 1 {
		return job.Errorf("Usage: %s container_id", job.Name)
	}
	var (
		name      = job.Args[0]
		container = daemon.Get(name)
	)

	if container == nil {
		return job.Errorf("No such container: %s", name)
	}

	if container.IsRunning() {
		return job.Errorf("Container already started")
	}

	// If no environment was set, then no hostconfig was passed.
	// This is kept for backward compatibility - hostconfig should be passed when
	// creating a container, not during start.
	if len(job.Environ()) > 0 {
		hostConfig := runconfig.ContainerHostConfigFromJob(job)
		if err := daemon.setHostConfig(container, hostConfig); err != nil {
			return job.Error(err)
		}
	}
	if err := container.Start(); err != nil {
		container.LogEvent("die")
		return job.Errorf("Cannot start container %s: %s", name, err)
	}

	return engine.StatusOK
}

func (daemon *Daemon) setHostConfig(container *Container, hostConfig *runconfig.HostConfig) error {
	// Validate the HostConfig binds. Make sure that:
	// the source exists
	for _, bind := range hostConfig.Binds {
		splitBind := strings.Split(bind, ":")
		source := splitBind[0]
		mountPoint := splitBind[1]
		if source == "/REMOTE" {
			log.Debugf("ContainerStart get /REMOTE")
			ch := make(chan net.Conn, 1)
			buf := make([]byte, 20)
			if _, err := cryptorand.Read(buf); err != nil {
				return err
			}

			handle := fmt.Sprintf("%x", buf)
			daemon.RegisterFSHandle(handle, ch)

			tmpName, err := ioutil.TempDir("", "")
			if err != nil {
				return err
			}
			log.Debugf("ContainerStart create server ->")
			s, err := vfuse.NewServer(tmpName, func() net.Conn {
				return <-ch
			})
			log.Debugf("<- ContainerStart create server")
			if err != nil {
				return fmt.Errorf("Starting fuse server: %v", err)
			}
			//TODO handle multiple fuse
			fmt.Fprintf(os.Stdout, "%s\n", handle)
			log.Debugf("ContainerStart run server")
			go s.Serve() //TODO call s.Unmount()
			//TODO cleanup tmp dir
			source = tmpName
			log.Debugf("ContainerStart update hostConfig")
			hostConfig.Binds[index] = fmt.Sprintf("%s:%s", source, mountPoint)
		} else {
			// ensure the source exists on the host
			_, err := os.Stat(source)
			if err != nil && os.IsNotExist(err) {
				err = os.MkdirAll(source, 0755)
				if err != nil {
					return fmt.Errorf("Could not create local directory '%s' for bind mount: %s!", source, err.Error())
				}
			}
		}
	}
	// Register any links from the host config before starting the container
	if err := daemon.RegisterLinks(container, hostConfig); err != nil {
		return err
	}
	container.SetHostConfig(hostConfig)
	container.ToDisk()

	return nil
}
