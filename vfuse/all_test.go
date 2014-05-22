// Integration tests for vfuse client & server.

package vfuse

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	verbose = flag.Bool("verbose", false, "verbose")
	broken  = flag.Bool("broken", false, "Run known-broken tests")
)

func knownBroken(t *testing.T) {
	if !*broken {
		t.Skip("skipping known-broken test")
	}
}

// This is the list of tests that call getWorld. It's used so we know
// how many tests will ultimately be run and when to do a best-effort
// cleanup on the release of a world, to remove temp files and such.
//
// TODO(bradfitz): consider generating this list automatically by
// using runtime.Stack and finding the goroutine in testing.Main and
// finding the argument with the slice of InternalTest and using some
// strconv and unsafe. That would be gross and awesome.
var worldlyTests []string

func isWorldTest(name string) bool {
	for _, n := range worldlyTests {
		if n == name {
			return true
		}
	}
	return false
}

// addWorldTest registers a test name that might call getWorld.
func addWorldTest(name string) {
	if !strings.HasPrefix(name, "Test") {
		panic("bogus registration of non-Test")
	}
	if isWorldTest(name) {
		panic("duplicate registration of " + name)
	}
	worldlyTests = append(worldlyTests, name)
}

func currentTestName() string {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		panic("Caller failed")
	}
	f := runtime.FuncForPC(pc)
	if f == nil {
		panic("can't find FuncForPC of test caller")
	}
	testName := f.Name()
	i := strings.Index(testName, "Test")
	if i < 0 {
		panic("unexpected test name: " + testName)
	}
	return testName[i:]
}

func getWorld(t *testing.T) *world {
	if runtime.GOOS != "linux" {
		t.Skip("test only runs on linux")
	}
	if n := currentTestName(); !isWorldTest(n) {
		t.Fatalf("getWorld called from %v which was not registered with addWorldTest", n)
	}

	currentTest = t
	if w := singleWorld; w != nil {
		w.t = t
		return w
	}
	testsToRun = countTestsToRun()
	singleWorld = newWorld(t)
	singleWorld.t = t
	return singleWorld
}

func countTestsToRun() int {
	f := flag.Lookup("test.run")
	if f == nil || f.Value.String() == "" {
		return len(worldlyTests)
	}
	rx, err := regexp.Compile(f.Value.String())
	if err != nil {
		// Shouldn't get this far anyway.
		return len(worldlyTests)
	}
	n := 0
	for _, name := range worldlyTests {
		if rx.MatchString(name) {
			n++
		}
	}
	return n
}

var (
	testsToRun  int
	worldsEnded int

	currentTest *testing.T
	singleWorld *world
)

type world struct {
	t *testing.T // changed per test. rest is static.

	port      int
	binDir    string
	mountDir  string
	clientDir string

	server      *exec.Cmd
	serverStdin io.WriteCloser

	client *exec.Cmd
}

func newWorld(t *testing.T) *world {
	w := &world{
		binDir:    tempDir(t, "bin"),
		mountDir:  tempDir(t, "mount"),
		clientDir: tempDir(t, "client"),
		port:      7070, // TODO: auto-pick a free one
	}

	vfused := filepath.Join(w.binDir, "vfused")
	out, err := exec.Command("go", "build", "-o", vfused, "github.com/dotcloud/docker/vfuse/vfused").CombinedOutput()
	if err != nil {
		t.Fatalf("vfused build failure: %v, %s", err, out)
	}
	vclient := filepath.Join(w.binDir, "vclient")
	out, err = exec.Command("go", "build", "-o", vclient, "github.com/dotcloud/docker/vfuse/client").CombinedOutput()
	if err != nil {
		t.Fatalf("client build failure: %v, %s", err, out)
	}

	w.server = exec.Command(vfused,
		"--mount="+w.mountDir,
		"--listen="+strconv.Itoa(w.port),
		"--verbose="+strconv.FormatBool(*verbose),
	)
	if *verbose {
		w.server.Stdout = os.Stdout
		w.server.Stderr = os.Stderr
	}
	sin, err := w.server.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	w.serverStdin = sin
	if err := w.server.Start(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300; i++ {
		if isMounted(w.mountDir) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !isMounted(w.mountDir) {
		t.Fatal("never saw %s get mounted", w.mountDir)
	}

	w.client = exec.Command(vclient,
		"--addr=localhost:"+strconv.Itoa(w.port),
		"--verbose="+strconv.FormatBool(*verbose),
	)
	w.client.Stdout = os.Stdout
	w.client.Stderr = os.Stderr
	w.client.Dir = w.clientDir
	if err := w.client.Start(); err != nil {
		t.Fatal(err)
	}

	return w
}

// fpath wraps filepath.Join(w.fuseMountDir, path...).
func (w *world) fpath(path ...string) string { return w.pathJoin(w.mountDir, path) }

// fpath wraps filepath.Join(w.clientDir, path...).
func (w *world) cpath(path ...string) string { return w.pathJoin(w.clientDir, path) }

func (w *world) pathJoin(base string, path []string) string {
	arg := make([]string, 0, len(path)+1)
	arg = append(arg, base)
	arg = append(arg, path...)
	return filepath.Join(arg...)
}

func (w *world) mkdir(path string) {
	if err := os.MkdirAll(path, 0755); err != nil {
		w.t.Fatalf("Error making dir %s: %v", path, err)
	}
}

func (w *world) writeFile(path string, contents string) {
	w.mkdir(filepath.Dir(path))
	if err := ioutil.WriteFile(path, []byte(contents), 0644); err != nil {
		w.t.Fatalf("Error writing %s: %v", path, err)
	}
}

func (w *world) release() {
	worldsEnded++
	if worldsEnded < testsToRun {
		return
	}
	if worldsEnded > testsToRun {
		w.t.Fatalf("unexpected number of releases called on world. forget to register in worldlyTests?")
	}
	w.t.Logf("(end of all tests; shutting down world)")

	w.client.Process.Kill()
	w.serverStdin.Write([]byte("q\n")) // tell FUSE server to close nicely
	w.server.Wait()                    // TODO(bradfitz): in a goroutine racing against a time limit?

	exec.Command("fusermount", "-u", w.mountDir).Run() // just in case

	removeAll(w.binDir)
	removeAll(w.mountDir)
	removeAll(w.clientDir)
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

func removeAll(path string) {
	if path == "" {
		panic("removeAll of empty string?")
	}
	os.RemoveAll(path) // best effort: just a tempdir
}

// Stat a regular file.
func init() { addWorldTest("TestStatRegular") }
func TestStatRegular(t *testing.T) {
	w := getWorld(t)
	defer w.release()

	const contents = "Some file contents.\n"
	const file = "stat_reg/file.txt"
	w.writeFile(w.cpath(file), contents)

	fi, err := os.Lstat(w.fpath(file))
	if err != nil {
		t.Fatalf("Lstat = %v; want valid file", err)
	}
	if fi.Size() != int64(len(contents)) {
		t.Errorf("stat size = %d; want %d", fi.Size(), len(contents))
	}
	if !fi.Mode().IsRegular() {
		t.Errorf("file isn't regular")
	}
}

// Stat a non-existant file.
func init() { addWorldTest("TestStatNoExist") }
func TestStatNoExist(t *testing.T) {
	w := getWorld(t)
	defer w.release()

	if _, err := os.Lstat(w.fpath("file-no-exist.txt")); !os.IsNotExist(err) {
		t.Errorf("For non-existant file, want os.IsNotExist; got err = %v", err)
	}
}

// Stat a directory.
func init() { addWorldTest("TestStatDir") }
func TestStatDir(t *testing.T) {
	w := getWorld(t)
	defer w.release()

	w.mkdir(w.cpath("stat_dir"))

	fi, err := os.Lstat(w.fpath("stat_dir"))
	if err != nil {
		t.Fatalf("Lstat = %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("Mode = %v; want Dir", fi.Mode())
	}
}

// Stat a symlink.
func init() { addWorldTest("TestStatSymlink") }
func TestStatSymlink(t *testing.T) {
	w := getWorld(t)
	defer w.release()

	w.mkdir(w.cpath("stat_symlink"))
	if err := os.Symlink("some-target", w.cpath("stat_symlink/link")); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Lstat(w.fpath("stat_symlink/link"))
	if err != nil {
		t.Fatalf("Lstat = %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("Mode = %v; want symlink bit", fi.Mode())
	}
}

// Readlink a symlink.
func init() { addWorldTest("TestReadlink") }
func TestReadlink(t *testing.T) {
	w := getWorld(t)
	defer w.release()

	const target = "some-target"
	w.mkdir(w.cpath("readlink"))
	if err := os.Symlink(target, w.cpath("readlink/link")); err != nil {
		t.Fatal(err)
	}

	got, err := os.Readlink(w.fpath("readlink/link"))
	if err != nil {
		t.Fatalf("Readlink = %v", err)
	}
	if got != target {
		t.Errorf("Readlink = %q; want %q", got, target)
	}
}

// Readdirnames on empty dir
func init() { addWorldTest("TestReaddirnamesEmpty") }
func TestReaddirnamesEmpty(t *testing.T) {
	w := getWorld(t)
	defer w.release()

	const dir = "readdir_empty"
	w.mkdir(w.cpath(dir))

	f, err := os.Open(w.fpath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("Readdirnames = %q; want empty", names)
	}
	if err := f.Close(); err != nil {
		t.Error(err)
	}
}

// Readdirnames on non-empty dir
func init() { addWorldTest("TestReaddirnames") }
func TestReaddirnames(t *testing.T) {
	w := getWorld(t)
	defer w.release()

	w.writeFile(w.cpath("dirnames/1.txt"), "file one")
	w.writeFile(w.cpath("dirnames/2.txt"), "file two")

	f, err := os.Open(w.fpath("dirnames"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	want := []string{"1.txt", "2.txt"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("Readdirnames = %q; want %q", names, want)
	}
	if err := f.Close(); err != nil {
		t.Error(err)
	}
}

// Readdirnames on non-empty dir
func init() { addWorldTest("TestReaddirWalk") }
func TestReaddirWalk(t *testing.T) {
	w := getWorld(t)
	defer w.release()

	w.writeFile(w.cpath("dirwalk/1.txt"), "one")
	w.writeFile(w.cpath("dirwalk/sub/2.txt"), "and two")

	var got bytes.Buffer
	err := filepath.Walk(w.fpath("dirwalk"), func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(w.fpath("."), path)
		if err != nil {
			t.Fatalf("Rel: %v", err)
		}
		fmt.Fprintf(&got, "%v = %v", filepath.ToSlash(rel), fi.Mode())
		if fi.Mode().IsRegular() {
			fmt.Fprintf(&got, " (size %d)", fi.Size())
		}
		got.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatalf("Walk error: %v", err)
	}
	want := `dirwalk = drwxr-x---
dirwalk/1.txt = -rw-r----- (size 3)
dirwalk/sub = drwxr-x---
dirwalk/sub/2.txt = -rw-r----- (size 7)
`
	if got.String() != want {
		t.Errorf("Walk got:\n%s\n\nWant:\n%s", got.String(), want)
	}
}
