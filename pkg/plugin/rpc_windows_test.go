package plugin

import (
	"testing"
	"time"

	stashExec "github.com/stashapp/stash/pkg/exec"
	"github.com/stashapp/stash/pkg/plugin/common"
	"golang.org/x/sys/windows"
)

// Windows has no interrupt signal to ignore.
func ignoreInterrupt() {}

// processGone reports whether the process pid exits within d.
func processGone(t *testing.T, pid int, d time.Duration) bool {
	t.Helper()
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		// no such process any more
		return true
	}
	defer func() { _ = windows.CloseHandle(h) }()
	ev, err := windows.WaitForSingleObject(h, uint32(d.Milliseconds()))
	if err != nil {
		t.Fatal(err)
	}
	return ev == windows.WAIT_OBJECT_0
}

// #2143: an RPC plugin is started like other helpers, so it ends when Stash
// exits. StopAll ends every helper Stash started, as happens when it exits.
func TestRPCPluginEndsWithStash(t *testing.T) {
	pipe, client := startTestRPCPlugin(t, "serve")

	// a run that doesn't finish by itself
	client.Go("RPCRunner.Run", common.PluginInput{Args: common.ArgsMap{"wait": "1"}}, &common.PluginOutput{}, nil)
	pid := pipe.cmd.Process.Pid
	if processGone(t, pid, 300*time.Millisecond) {
		t.Fatal("the plugin exited early")
	}

	stashExec.StopAll()

	if !processGone(t, pid, 5*time.Second) {
		t.Error("the plugin kept running after Stash's helpers were ended")
	}
	_ = client.Close()
}
