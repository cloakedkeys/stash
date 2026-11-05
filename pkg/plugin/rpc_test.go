package plugin

import (
	"fmt"
	"io"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/plugin/common"
)

// The test binary stands in for an RPC plugin when this is set.
const rpcTestPluginEnv = "STASH_RPC_TEST_PLUGIN"

func TestMain(m *testing.M) {
	switch os.Getenv(rpcTestPluginEnv) {
	case "":
		os.Exit(m.Run())
	case "serve":
		if err := common.ServePlugin(&testRPCRunner{}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	case "ignore":
		// a plugin that doesn't exit when its stdin is closed, or on an
		// interrupt
		ignoreInterrupt()
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}

type testRPCRunner struct{}

func (*testRPCRunner) Run(input common.PluginInput, output *common.PluginOutput) error {
	if input.Args.String("wait") != "" {
		time.Sleep(time.Minute)
	}
	s := "hello " + input.Args.String("name")
	output.Output = s
	return nil
}

func (*testRPCRunner) Stop(input struct{}, output *bool) error {
	return nil
}

func startTestRPCPlugin(t *testing.T, mode string) (*rpcPipe, *rpc.Client) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(rpcTestPluginEnv, mode)
	pipe, err := startRPCPipe(io.Discard, exe)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pipe.cmd.Process.Kill() })
	return pipe, rpc.NewClientWithCodec(jsonrpc.NewClientCodec(pipe))
}

// waitExited reports whether the plugin's process exits within d.
func waitExited(pipe *rpcPipe, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if pipe.cmd.ProcessState != nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return pipe.cmd.ProcessState != nil
}

func TestRPCPluginRuns(t *testing.T) {
	pipe, client := startTestRPCPlugin(t, "serve")

	var out common.PluginOutput
	input := common.PluginInput{Args: common.ArgsMap{"name": "stash"}}
	if err := (rpcPluginClient{Client: client}).Run(input, &out); err != nil {
		t.Fatal(err)
	}
	if out.Output != "hello stash" {
		t.Errorf("output %v, want hello stash", out.Output)
	}

	if err := client.Close(); err != nil {
		t.Logf("close: %v", err)
	}
	if !waitExited(pipe, 5*time.Second) {
		t.Error("the plugin kept running after it was closed")
	}
}

// A plugin that ignores having its pipes closed is killed after the timeout.
func TestRPCPluginKilledAfterTimeout(t *testing.T) {
	old := rpcStopTimeout
	rpcStopTimeout = 200 * time.Millisecond
	t.Cleanup(func() { rpcStopTimeout = old })

	pipe, client := startTestRPCPlugin(t, "ignore")
	time.Sleep(200 * time.Millisecond) // let it start ignoring the interrupt

	start := time.Now()
	_ = client.Close()
	if !waitExited(pipe, 5*time.Second) {
		t.Fatal("the plugin wasn't killed")
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("closing took %v", took)
	}
}
