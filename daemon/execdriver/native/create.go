// +build linux,cgo

package native

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/daemon/execdriver"
	"github.com/docker/docker/daemon/execdriver/native/template"
	"github.com/docker/docker/pkg/symlink"
	"github.com/docker/libcontainer/apparmor"
	"github.com/docker/libcontainer/configs"
	"github.com/docker/libcontainer/devices"
	"github.com/docker/libcontainer/utils"
)

// createContainer populates and configures the container type with the
// data provided by the execdriver.Command
func (d *driver) createContainer(c *execdriver.Command) (*configs.Config, error) {
	container := template.New()

	container.Hostname = getEnv("HOSTNAME", c.ProcessConfig.Env)
	container.Cgroups.Name = c.ID
	container.Cgroups.AllowedDevices = c.AllowedDevices
	container.Devices = c.AutoCreatedDevices
	container.Rootfs = c.Rootfs
	container.Readonlyfs = c.ReadonlyRootfs

	// check to see if we are running in ramdisk to disable pivot root
	container.NoPivotRoot = os.Getenv("DOCKER_RAMDISK") != ""

	if err := d.createIpc(container, c); err != nil {
		return nil, err
	}

	if err := d.createPid(container, c); err != nil {
		return nil, err
	}

	if err := d.createNetwork(container, c); err != nil {
		return nil, err
	}

	if c.ProcessConfig.Privileged {
		// clear readonly for /sys
		container.Mounts[2].Flags &= ^syscall.MS_RDONLY
		if err := d.setPrivileged(container); err != nil {
			return nil, err
		}
	} else {
		if err := d.setCapabilities(container, c); err != nil {
			return nil, err
		}
	}

	if c.AppArmorProfile != "" {
		container.AppArmorProfile = c.AppArmorProfile
	}

	if err := d.setupCgroups(container, c); err != nil {
		return nil, err
	}

	if err := d.setupMounts(container, c); err != nil {
		return nil, err
	}

	if err := d.setupLabels(container, c); err != nil {
		return nil, err
	}

	return container, nil
}

func generateIfaceName() (string, error) {
	for i := 0; i < 10; i++ {
		name, err := utils.GenerateRandomName("veth", 7)
		if err != nil {
			continue
		}
		if _, err := net.InterfaceByName(name); err != nil {
			if strings.Contains(err.Error(), "no such") {
				return name, nil
			}
			return "", err
		}
	}
	return "", errors.New("Failed to find name for new interface")
}

func (d *driver) createNetwork(container *configs.Config, c *execdriver.Command) error {
	if c.Network.HostNetworking {
		container.Namespaces.Remove(configs.NEWNET)
		return nil
	}

	container.Networks = []*configs.Network{
		{
			Type: "loopback",
		},
	}

	iName, err := generateIfaceName()
	if err != nil {
		return err
	}
	if c.Network.Interface != nil {
		vethNetwork := configs.Network{
			Name:              "eth0",
			HostInterfaceName: iName,
			Mtu:               c.Network.Mtu,
			Address:           fmt.Sprintf("%s/%d", c.Network.Interface.IPAddress, c.Network.Interface.IPPrefixLen),
			MacAddress:        c.Network.Interface.MacAddress,
			Gateway:           c.Network.Interface.Gateway,
			Type:              "veth",
			Bridge:            c.Network.Interface.Bridge,
		}
		if c.Network.Interface.GlobalIPv6Address != "" {
			vethNetwork.IPv6Address = fmt.Sprintf("%s/%d", c.Network.Interface.GlobalIPv6Address, c.Network.Interface.GlobalIPv6PrefixLen)
			vethNetwork.IPv6Gateway = c.Network.Interface.IPv6Gateway
		}
		container.Networks = append(container.Networks, &vethNetwork)
	}

	if c.Network.ContainerID != "" {
		d.Lock()
		active := d.activeContainers[c.Network.ContainerID]
		d.Unlock()

		if active == nil {
			return fmt.Errorf("%s is not a valid running container to join", c.Network.ContainerID)
		}

		state, err := active.State()
		if err != nil {
			return err
		}

		container.Namespaces.Add(configs.NEWNET, state.NamespacePaths[configs.NEWNET])
	}

	return nil
}

func (d *driver) createIpc(container *configs.Config, c *execdriver.Command) error {
	if c.Ipc.HostIpc {
		container.Namespaces.Remove(configs.NEWIPC)
		return nil
	}

	if c.Ipc.ContainerID != "" {
		d.Lock()
		active := d.activeContainers[c.Ipc.ContainerID]
		d.Unlock()

		if active == nil {
			return fmt.Errorf("%s is not a valid running container to join", c.Ipc.ContainerID)
		}

		state, err := active.State()
		if err != nil {
			return err
		}
		container.Namespaces.Add(configs.NEWIPC, state.NamespacePaths[configs.NEWIPC])
	}

	return nil
}

func (d *driver) createPid(container *configs.Config, c *execdriver.Command) error {
	if c.Pid.HostPid {
		container.Namespaces.Remove(configs.NEWPID)
		return nil
	}

	return nil
}

func (d *driver) setPrivileged(container *configs.Config) (err error) {
	container.Capabilities = execdriver.GetAllCapabilities()
	container.Cgroups.AllowAllDevices = true

	hostDevices, err := devices.HostDevices()
	if err != nil {
		return err
	}
	container.Devices = hostDevices

	if apparmor.IsEnabled() {
		container.AppArmorProfile = "unconfined"
	}

	return nil
}

func (d *driver) setCapabilities(container *configs.Config, c *execdriver.Command) (err error) {
	container.Capabilities, err = execdriver.TweakCapabilities(container.Capabilities, c.CapAdd, c.CapDrop)
	return err
}

func (d *driver) setupCgroups(container *configs.Config, c *execdriver.Command) error {
	if c.Resources != nil {
		container.Cgroups.CpuShares = c.Resources.CpuShares
		container.Cgroups.Memory = c.Resources.Memory
		container.Cgroups.MemoryReservation = c.Resources.Memory
		container.Cgroups.MemorySwap = c.Resources.MemorySwap
		container.Cgroups.CpusetCpus = c.Resources.Cpuset
	}

	return nil
}

func (d *driver) setupMounts(container *configs.Config, c *execdriver.Command) error {
	for _, m := range c.Mounts {
		dest, err := symlink.FollowSymlinkInScope(filepath.Join(c.Rootfs, m.Destination), c.Rootfs)
		if err != nil {
			return err
		}
		flags := syscall.MS_BIND | syscall.MS_REC
		if !m.Writable {
			flags |= syscall.MS_RDONLY
		}
		if m.Slave {
			flags |= syscall.MS_SLAVE
		}

		container.Mounts = append(container.Mounts, &configs.Mount{
			Source:      m.Source,
			Destination: dest,
			Device:      "bind",
			Flags:       flags,
		})
	}
	for _, m := range container.Mounts {
		logrus.Errorf("Mount, Source: %s, Dest: %s", m.Source, m.Destination)
	}

	return nil
}

func (d *driver) setupLabels(container *configs.Config, c *execdriver.Command) error {
	container.ProcessLabel = c.ProcessLabel
	container.MountLabel = c.MountLabel

	return nil
}
