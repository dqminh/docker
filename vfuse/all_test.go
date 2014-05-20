package vfuse

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestAll(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test only runs on linux")
	}
	binDir := tempDir(t, "bin")
	defer os.RemoveAll(binDir)
	mountDir := tempDir(t, "mount")
	defer os.RemoveAll(mountDir)
	clientDir := tempDir(t, "client")
	defer os.RemoveAll(clientDir)
	defer exec.Command("fusermount", "-u", mountDir).Run()

	vfused := filepath.Join(binDir, "vfused")
	out, err := exec.Command("go", "build", "-o", vfused, "github.com/dotcloud/docker/vfuse/vfused").CombinedOutput()
	if err != nil {
		t.Fatalf("vfused build failure: %v, %s", err, out)
	}
	vclient := filepath.Join(binDir, "vclient")
	out, err = exec.Command("go", "build", "-o", vclient, "github.com/dotcloud/docker/vfuse/client").CombinedOutput()
	if err != nil {
		t.Fatalf("client build failure: %v, %s", err, out)
	}

	const fileContents = "Some file contents.\n"
	if err := ioutil.WriteFile(filepath.Join(clientDir, "File.txt"), []byte(fileContents), 0644); err != nil {
		t.Fatal(err)
	}

	port := 7070
	serverCmd := exec.Command(vfused, "--mount="+mountDir, "--listen="+strconv.Itoa(port))
	serverCmd.Stdout = os.Stdout
	serverCmd.Stderr = os.Stderr
	sin, err := serverCmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := serverCmd.Start(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300; i++ {
		if isMounted(mountDir) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !isMounted(mountDir) {
		t.Fatal("never saw %s get mounted", mountDir)
	}

	clientCmd := exec.Command(vclient, "--addr=localhost:"+strconv.Itoa(port))
	clientCmd.Stdout = os.Stdout
	clientCmd.Stderr = os.Stderr
	clientCmd.Dir = clientDir
	if err := clientCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer clientCmd.Process.Kill()

	fi, err := os.Lstat(filepath.Join(mountDir, "File.txt"))
	if err != nil {
		t.Fatalf("File.txt Lstat = %v; want valid file", err)
	}
	if fi.Size() != int64(len(fileContents)) {
		t.Errorf("File.txt stat size = %d; want %d", fi.Size(), len(fileContents))
	}
	if !fi.Mode().IsRegular() {
		t.Errorf("File.txt isn't regular")
	}

	sin.Write([]byte("q\n")) // tell FUSE server to close nicely
	serverCmd.Wait()
}

func isMounted(dir string) bool {
	slurp, _ := ioutil.ReadFile("/proc/mounts")
	return bytes.Contains(slurp, []byte(dir)) // close enough.
}

func tempDir(t *testing.T, name string) string {
	dir, err := ioutil.TempDir("", name)
	if err != nil {
		t.Fatalf("Error making temp dir: %v", err)
	}
	return dir
}
