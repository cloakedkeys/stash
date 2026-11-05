//go:build windows

package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const helperEnv = "STASH_EXEC_TEST_HELPER"

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	user32                    = windows.NewLazySystemDLL("user32.dll")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	procGetConsoleWindow      = kernel32.NewProc("GetConsoleWindow")
	procAllocConsole          = kernel32.NewProc("AllocConsole")
	procIsWindowVisible       = user32.NewProc("IsWindowVisible")
	procShowWindow            = user32.NewProc("ShowWindow")
)

// consoleProcesses returns the ids of the processes attached to the calling
// process's console, or nil if it has no console.
func consoleProcesses() []uint32 {
	pids := make([]uint32, 256)
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n == 0 || int(n) > len(pids) {
		return nil
	}
	return pids[:n]
}

// consoleWindowVisible reports whether the calling process's console has a
// visible window.
func consoleWindowVisible() bool {
	h, _, _ := procGetConsoleWindow.Call()
	if h == 0 {
		return false
	}
	v, _, _ := procIsWindowVisible.Call(h)
	return v != 0
}

// describeConsole is the line a helper process prints about itself.
func describeConsole() string {
	var ids []string
	for _, pid := range consoleProcesses() {
		ids = append(ids, strconv.FormatUint(uint64(pid), 10))
	}
	return fmt.Sprintf("pid=%d visible=%v console=%s", os.Getpid(), consoleWindowVisible(), strings.Join(ids, ","))
}

type consoleInfo struct {
	pid     uint32
	visible bool
	console []uint32
}

func parseConsoleInfo(t *testing.T, line string) consoleInfo {
	t.Helper()
	var info consoleInfo
	for _, field := range strings.Fields(line) {
		k, v, _ := strings.Cut(field, "=")
		switch k {
		case "pid":
			n, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				t.Fatalf("bad helper line %q", line)
			}
			info.pid = uint32(n)
		case "visible":
			info.visible = v == "true"
		case "console":
			for _, s := range strings.Split(v, ",") {
				if s == "" {
					continue
				}
				n, err := strconv.ParseUint(s, 10, 32)
				if err != nil {
					t.Fatalf("bad helper line %q", line)
				}
				info.console = append(info.console, uint32(n))
			}
		}
	}
	return info
}

func contains(pids []uint32, pid uint32) bool {
	for _, p := range pids {
		if p == pid {
			return true
		}
	}
	return false
}

// TestHelperProcess is not a real test. The other tests run the test binary
// again through Command, and this function does the child's work.
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv(helperEnv)
	if mode == "" {
		t.Skip("helper process only")
	}
	os.Exit(runHelper(mode))
}

func runHelper(mode string) int {
	switch mode {
	case "console":
		fmt.Println(describeConsole())
		// Start a grandchild the way a plugin would, with no special flags.
		gc := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
		gc.Env = append(os.Environ(), helperEnv+"=grandchild")
		out, err := gc.Output()
		if err != nil {
			fmt.Println("grandchild error:", err)
			return 1
		}
		fmt.Print(string(out))
	case "grandchild":
		fmt.Println(describeConsole())
	case "echo":
		in, err := io.ReadAll(os.Stdin)
		if err != nil {
			return 1
		}
		fmt.Fprint(os.Stdout, "out:"+string(in))
		fmt.Fprint(os.Stderr, "err:"+string(in))
		return 3
	case "sleep":
		time.Sleep(time.Minute)
	case "spawn":
		// Start a long-lived grandchild the way a plugin would, report its
		// pid, then wait.
		gc := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
		gc.Env = append(os.Environ(), helperEnv+"=sleep")
		if err := gc.Start(); err != nil {
			fmt.Println("grandchild error:", err)
			return 1
		}
		fmt.Println(gc.Process.Pid)
		time.Sleep(time.Minute)
	case "owner":
		// Act as Stash: start a helper that starts a grandchild, report both
		// pids, then exit without stopping either.
		child := helperCommand(context.Background(), "spawn")
		stdout, err := child.StdoutPipe()
		if err != nil {
			return 1
		}
		if err := Start(child); err != nil {
			fmt.Println("start error:", err)
			return 1
		}
		var grandchild int
		if _, err := fmt.Fscan(stdout, &grandchild); err != nil {
			fmt.Println("read error:", err)
			return 1
		}
		fmt.Println(child.Process.Pid, grandchild)
		return 0
	case "fail":
		fmt.Fprint(os.Stdout, "partial")
		fmt.Fprint(os.Stderr, "something went wrong")
		return 2
	}
	return 0
}

func helperCommand(ctx context.Context, mode string) *exec.Cmd {
	cmd := CommandContext(ctx, os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Env = append(os.Environ(), helperEnv+"="+mode)
	return cmd
}

// ensureConsole gives the test process a console if it has none, so there is
// a console the child could wrongly share.
func ensureConsole(t *testing.T) {
	t.Helper()
	if consoleProcesses() != nil {
		return
	}
	if r, _, err := procAllocConsole.Call(); r == 0 {
		t.Skipf("no console and AllocConsole failed: %v", err)
	}
	if h, _, _ := procGetConsoleWindow.Call(); h != 0 {
		const swHide = 0
		_, _, _ = procShowWindow.Call(h, swHide)
	}
}

func TestCommandDoesNotShareConsole(t *testing.T) {
	ensureConsole(t)
	parent := uint32(os.Getpid())
	if !contains(consoleProcesses(), parent) {
		t.Fatalf("test process is not on its own console list")
	}

	out, err := Output(helperCommand(context.Background(), "console"))
	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected a child and a grandchild line, got %q", out)
	}
	child := parseConsoleInfo(t, lines[0])
	grandchild := parseConsoleInfo(t, lines[1])

	// The child has a console, but not Stash's, so Stash's Ctrl-C and
	// Ctrl-Break don't reach it.
	if len(child.console) == 0 {
		t.Errorf("child has no console; console programs it starts would open windows")
	}
	if contains(child.console, parent) {
		t.Errorf("child shares the parent's console: %v", child.console)
	}
	if child.visible {
		t.Errorf("child has a visible console window")
	}

	// A process the child starts itself shares the child's hidden console,
	// rather than getting a new window.
	if !contains(grandchild.console, child.pid) {
		t.Errorf("grandchild is not on the child's console: %v", grandchild.console)
	}
	if contains(grandchild.console, parent) {
		t.Errorf("grandchild shares the parent's console: %v", grandchild.console)
	}
	if grandchild.visible {
		t.Errorf("grandchild has a visible console window")
	}
}

func TestCommandPipes(t *testing.T) {
	cmd := helperCommand(context.Background(), "echo")
	cmd.Stdin = strings.NewReader("hello")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := Run(cmd)
	var exitErr *exec.ExitError
	if err == nil || !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("expected exit code 3, got %v", err)
	}
	if stdout.String() != "out:hello" {
		t.Errorf("stdout = %q", stdout.String())
	}
	if stderr.String() != "err:hello" {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestCommandContextCancelStopsChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := helperCommand(ctx, "sleep")
	if err := Start(cmd); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Errorf("expected an error from a cancelled command")
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("cancelling the context did not stop the child")
	}
}

func TestCommandKillStopsChild(t *testing.T) {
	cmd := helperCommand(context.Background(), "sleep")
	if err := Start(cmd); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Kill did not stop the child")
	}
}
