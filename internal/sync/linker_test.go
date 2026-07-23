package sync

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/GyroZepelix/loremaster/internal/manifest"
)

func TestLinkItemSoftFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.md")
	dst := filepath.Join(dir, "target", "review.md")
	if err := os.WriteFile(src, []byte("review"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LinkItem(src, dst, "soft", nil)
	if err != nil {
		t.Fatalf("LinkItem: %v", err)
	}
	if result.Kind != "file" || result.Mode != "soft" {
		t.Fatalf("result = %#v", result)
	}
	if info, err := os.Lstat(dst); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("destination is not a symlink: %v", err)
	}
	content, err := os.ReadFile(dst)
	if err != nil || string(content) != "review" {
		t.Fatalf("content = %q, err = %v", content, err)
	}
}

func TestLinkItemHardFileAndDirectory(t *testing.T) {
	dir := t.TempDir()
	fileSrc := filepath.Join(dir, "source.json")
	if err := os.WriteFile(fileSrc, []byte(`{"ok":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	fileDst := filepath.Join(dir, "out", "tool.json")
	fileResult, err := LinkItem(fileSrc, fileDst, "hard", nil)
	if err != nil {
		t.Fatalf("hard file: %v", err)
	}
	if fileResult.Kind != "file" || fileResult.Checksum == "" {
		t.Fatalf("file result = %#v", fileResult)
	}

	dirSrc := filepath.Join(dir, "source-dir")
	if err := os.MkdirAll(filepath.Join(dirSrc, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirSrc, "nested", "value.txt"), []byte("value"), 0644); err != nil {
		t.Fatal(err)
	}
	dirDst := filepath.Join(dir, "out", "bundle")
	dirResult, err := LinkItem(dirSrc, dirDst, "hard", nil)
	if err != nil {
		t.Fatalf("hard directory: %v", err)
	}
	if dirResult.Kind != "directory" || dirResult.Checksum == "" {
		t.Fatalf("directory result = %#v", dirResult)
	}
	if _, err := os.Stat(filepath.Join(dirDst, ".lore-checksum")); !os.IsNotExist(err) {
		t.Fatalf("new hard directory contains legacy checksum marker: %v", err)
	}
}

func TestLinkItemRefusesReplacedManagedSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.md")
	other := filepath.Join(dir, "other.md")
	dst := filepath.Join(dir, "review.md")
	os.WriteFile(src, []byte("source"), 0644)
	os.WriteFile(other, []byte("other"), 0644)
	first, err := LinkItem(src, dst, "soft", nil)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(dst)
	os.Symlink(other, dst)
	state := &ManagedState{Mode: first.Mode, Kind: first.Kind, Target: first.Target}
	if _, err := LinkItem(src, dst, "soft", state); err == nil || !strings.Contains(err.Error(), "was replaced") {
		t.Fatalf("error = %v", err)
	}
	if target, _ := os.Readlink(dst); target != other {
		t.Fatalf("replaced symlink changed to %q", target)
	}
}

func TestLinkItemFailurePreservesManagedDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.md")
	dst := filepath.Join(dir, "review.md")
	os.WriteFile(src, []byte("source"), 0644)
	first, err := LinkItem(src, dst, "soft", nil)
	if err != nil {
		t.Fatal(err)
	}
	state := &ManagedState{Mode: first.Mode, Kind: first.Kind, Target: first.Target}
	os.Remove(src)
	if _, err := LinkItem(src, dst, "soft", state); err == nil {
		t.Fatal("expected missing-source error")
	}
	if target, err := os.Readlink(dst); err != nil || target != first.Target {
		t.Fatalf("managed destination was not preserved: target=%q err=%v", target, err)
	}
}

func TestTransactionalReplacementRollsBack(t *testing.T) {
	dir := t.TempDir()
	oldSource := filepath.Join(dir, "old.md")
	newSource := filepath.Join(dir, "new.md")
	dst := filepath.Join(dir, "review.md")
	os.WriteFile(oldSource, []byte("old"), 0644)
	os.WriteFile(newSource, []byte("new"), 0644)
	first, err := LinkItem(oldSource, dst, "soft", nil)
	if err != nil {
		t.Fatal(err)
	}
	state := &ManagedState{Mode: first.Mode, Kind: first.Kind, Target: first.Target}
	replacement, err := LinkItemTransactional(newSource, dst, "soft", state)
	if err != nil || replacement.Change == nil {
		t.Fatalf("transactional replacement: result=%#v err=%v", replacement, err)
	}
	if target, _ := os.Readlink(dst); target != newSource {
		t.Fatalf("new item not installed: %q", target)
	}
	if errs := RollbackChanges([]Change{*replacement.Change}); len(errs) != 0 {
		t.Fatalf("rollback errors: %v", errs)
	}
	if target, _ := os.Readlink(dst); target != oldSource {
		t.Fatalf("old item not restored: %q", target)
	}
}

func TestTransactionalCreationRollsBack(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.md")
	dst := filepath.Join(dir, "review.md")
	os.WriteFile(src, []byte("review"), 0644)
	result, err := LinkItemTransactional(src, dst, "soft", nil)
	if err != nil || result.Change == nil {
		t.Fatalf("transactional creation: result=%#v err=%v", result, err)
	}
	if errs := RollbackChanges([]Change{*result.Change}); len(errs) != 0 {
		t.Fatalf("rollback errors: %v", errs)
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Fatalf("created item remains after rollback: %v", err)
	}
}

func TestLinkItemStagingFailurePreservesHardDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix socket failure injection is not available on Windows")
	}
	dir := t.TempDir()
	oldSource := filepath.Join(dir, "old")
	newSource := filepath.Join(dir, "new")
	dst := filepath.Join(dir, "installed")
	os.Mkdir(oldSource, 0755)
	os.WriteFile(filepath.Join(oldSource, "value.txt"), []byte("old"), 0644)
	first, err := LinkItem(oldSource, dst, "hard", nil)
	if err != nil {
		t.Fatal(err)
	}
	os.Mkdir(newSource, 0755)
	os.WriteFile(filepath.Join(newSource, "value.txt"), []byte("new"), 0644)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(newSource, "socket")})
	if err != nil {
		t.Skipf("cannot create unix socket: %v", err)
	}
	defer listener.Close()
	state := &ManagedState{Mode: first.Mode, Kind: first.Kind, Checksum: first.Checksum, ChecksumVersion: first.ChecksumVersion}
	if _, err := LinkItem(newSource, dst, "hard", state); err == nil {
		t.Fatal("expected staged copy failure")
	}
	content, err := os.ReadFile(filepath.Join(dst, "value.txt"))
	if err != nil || string(content) != "old" {
		t.Fatalf("old destination was not preserved: content=%q err=%v", content, err)
	}
}

func TestDirectoryChecksumIncludesDirectoriesAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("value"), 0644)
	base, err := ComputeDirChecksum(dir)
	if err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty")
	os.Mkdir(empty, 0755)
	withDirectory, _ := ComputeDirChecksum(dir)
	if withDirectory == base {
		t.Fatal("adding an empty directory did not change the checksum")
	}
	os.Remove(empty)
	link := filepath.Join(dir, "link")
	os.Symlink("file.txt", link)
	withSymlink, _ := ComputeDirChecksum(dir)
	if withSymlink == base {
		t.Fatal("adding a symlink did not change the checksum")
	}
	os.Remove(link)
	os.Symlink("other.txt", link)
	changedTarget, _ := ComputeDirChecksum(dir)
	if changedTarget == withSymlink {
		t.Fatal("changing a symlink target did not change the checksum")
	}
}

func TestHardFileChecksumProtectsPermissionChanges(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.sh")
	dst := filepath.Join(dir, "installed.sh")
	os.WriteFile(src, []byte("echo test\n"), 0644)
	first, err := LinkItem(src, dst, "hard", nil)
	if err != nil {
		t.Fatal(err)
	}
	os.Chmod(dst, 0755)
	state := &ManagedState{Mode: first.Mode, Kind: first.Kind, Checksum: first.Checksum, ChecksumVersion: first.ChecksumVersion}
	if _, err := LinkItem(src, dst, "hard", state); err == nil || !strings.Contains(err.Error(), "local modifications") {
		t.Fatalf("update error = %v", err)
	}
	item := manifest.Item{Path: ".claude/hooks/installed.sh", Provider: "claude", Resource: "hooks", Mode: "hard", Kind: "file", Checksum: first.Checksum, ChecksumVersion: first.ChecksumVersion}
	project := filepath.Join(dir, "project")
	managedPath := filepath.Join(project, item.Path)
	os.MkdirAll(filepath.Dir(managedPath), 0755)
	os.Rename(dst, managedPath)
	if err := RemoveManagedItem(project, item); err == nil || !strings.Contains(err.Error(), "local modifications") {
		t.Fatalf("removal error = %v", err)
	}
	if _, err := os.Stat(managedPath); err != nil {
		t.Fatalf("permission-modified file was removed: %v", err)
	}
}

func TestHardDirectoryChecksumProtectsLoreChecksumPayload(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	dst := filepath.Join(dir, "installed")
	os.Mkdir(src, 0755)
	os.WriteFile(filepath.Join(src, ".lore-checksum"), []byte("payload"), 0644)
	first, err := LinkItem(src, dst, "hard", nil)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dst, ".lore-checksum"), []byte("modified"), 0644)
	state := &ManagedState{Mode: first.Mode, Kind: first.Kind, Checksum: first.Checksum, ChecksumVersion: first.ChecksumVersion}
	if _, err := LinkItem(src, dst, "hard", state); err == nil || !strings.Contains(err.Error(), "local modifications") {
		t.Fatalf("error = %v", err)
	}
}

func TestLinkItemRefusesUnmanagedDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.md")
	dst := filepath.Join(dir, "review.md")
	os.WriteFile(src, []byte("new"), 0644)
	os.WriteFile(dst, []byte("user"), 0644)

	_, err := LinkItem(src, dst, "soft", nil)
	if err == nil || !strings.Contains(err.Error(), "not owned") {
		t.Fatalf("error = %v", err)
	}
	content, _ := os.ReadFile(dst)
	if string(content) != "user" {
		t.Fatalf("unmanaged destination changed: %q", content)
	}
}

func TestLinkItemManagedUpdatesAndModificationProtection(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.md")
	dst := filepath.Join(dir, "review.md")
	os.WriteFile(src, []byte("one"), 0644)

	first, err := LinkItem(src, dst, "hard", nil)
	if err != nil {
		t.Fatal(err)
	}
	state := &ManagedState{Mode: first.Mode, Kind: first.Kind, Checksum: first.Checksum}
	os.WriteFile(src, []byte("two"), 0644)
	second, err := LinkItem(src, dst, "hard", state)
	if err != nil {
		t.Fatalf("managed update: %v", err)
	}
	if second.Checksum == first.Checksum {
		t.Fatal("checksum did not change")
	}

	os.WriteFile(dst, []byte("local edit"), 0644)
	state = &ManagedState{Mode: second.Mode, Kind: second.Kind, Checksum: second.Checksum}
	if _, err := LinkItem(src, dst, "hard", state); err == nil || !strings.Contains(err.Error(), "local modifications") {
		t.Fatalf("modified destination error = %v", err)
	}
	content, _ := os.ReadFile(dst)
	if string(content) != "local edit" {
		t.Fatalf("modified destination changed: %q", content)
	}
}
