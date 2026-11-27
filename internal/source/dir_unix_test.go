//go:build unix

package source

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestDirLeavesOutPhotosItCannotRead runs a sync as an unprivileged user over
// files that belong to someone else: the private one can be neither taken
// over nor read, so it is left out and named in the error; the readable one
// stays, whatever its owner.
func TestDirLeavesOutPhotosItCannotRead(t *testing.T) {
	const asNobody = "PHOTOFRAME_TEST_SYNC_AS_NOBODY"
	if root := os.Getenv(asNobody); root != "" {
		photos, err := (&Dir{SourceID: "up", Root: root}).Sync(context.Background())
		var names []string
		for _, p := range photos {
			names = append(names, p.Name)
		}
		fmt.Printf("photos=%v\nerr=%v\n", names, err)
		return
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root, to run the sync as another user")
	}
	root := t.TempDir()
	img := pngBytes(t)
	write(t, filepath.Join(root, "private.jpg"), img)
	write(t, filepath.Join(root, "public.jpg"), img)
	if err := os.Chmod(filepath.Join(root, "private.jpg"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The test binary sits where only root may go; put a copy where the
	// other user can run it.
	self, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "sync.test")
	if err := os.WriteFile(bin, self, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, filepath.Dir(bin), filepath.Dir(root)} {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.Command(bin, "-test.run=^TestDirLeavesOutPhotosItCannotRead$")
	cmd.Env = []string{asNobody + "=" + root}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sync as another user: %v\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "photos=[public.jpg]\n") {
		t.Errorf("want only public.jpg listed, got:\n%s", got)
	}
	for _, want := range []string{"1 photo left out", "private.jpg", "operation not permitted"} {
		if !strings.Contains(got, want) {
			t.Errorf("error does not mention %q:\n%s", want, got)
		}
	}
}
