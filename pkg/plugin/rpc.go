package plugin

import (
	"errors"
	"fmt"
	"io"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	stashExec "github.com/stashapp/stash/pkg/exec"
	"github.com/stashapp/stash/pkg/plugin/common"
)

// rpcStopTimeout is how long a plugin has to exit once its pipes are closed,
// before it is killed. It is a variable to keep tests fast.
var rpcStopTimeout = time.Second

// startRPCPlugin starts an RPC plugin and returns a JSON-RPC client that talks
// to it over its stdin and stdout, writing its stderr to stderr. It does what
// pie.StartProviderCodec did, but starts the plugin through pkg/exec, so on
// Windows it gets its own hidden console and ends when Stash exits, as other
// helpers do.
func startRPCPlugin(stderr io.Writer, name string, args ...string) (*rpc.Client, error) {
	pipe, err := startRPCPipe(stderr, name, args...)
	if err != nil {
		return nil, err
	}
	return rpc.NewClientWithCodec(jsonrpc.NewClientCodec(pipe)), nil
}

// startRPCPipe starts an RPC plugin and returns its pipe.
func startRPCPipe(stderr io.Writer, name string, args ...string) (*rpcPipe, error) {
	cmd := stashExec.Command(name, args...)
	cmd.Stderr = stderr
	// don't wait forever for stderr if something the plugin started keeps it
	// open after the plugin has exited
	cmd.WaitDelay = rpcStopTimeout

	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	if err := stashExec.Start(cmd); err != nil {
		in.Close()
		out.Close()
		return nil, err
	}

	return &rpcPipe{ReadCloser: out, WriteCloser: in, cmd: cmd}, nil
}

// rpcPipe is the plugin's stdout and stdin, closed together with the plugin.
type rpcPipe struct {
	io.ReadCloser
	io.WriteCloser
	cmd *exec.Cmd
}

// Close closes the pipes, then waits for the plugin to exit, and kills it if
// it hasn't within rpcStopTimeout. Outside Windows the plugin is sent an
// interrupt first, as pie did. Windows has no interrupt signal; a pie plugin
// exits there once its stdin is closed.
func (p *rpcPipe) Close() error {
	err := p.ReadCloser.Close()
	if writeErr := p.WriteCloser.Close(); writeErr != nil {
		err = writeErr
	}

	exited := make(chan struct{})
	go func() {
		// the exit status of a plugin that is told to stop doesn't matter
		_ = p.cmd.Wait()
		close(exited)
	}()

	if runtime.GOOS != "windows" {
		_ = p.cmd.Process.Signal(os.Interrupt)
	}

	select {
	case <-exited:
	case <-time.After(rpcStopTimeout):
		if killErr := p.cmd.Process.Kill(); killErr != nil {
			return fmt.Errorf("error killing plugin after timeout: %w", killErr)
		}
		<-exited
	}
	return err
}

type rpcTaskBuilder struct{}

func (*rpcTaskBuilder) build(task pluginTask) Task {
	return &rpcPluginTask{
		pluginTask: task,
	}
}

type rpcPluginClient struct {
	Client *rpc.Client
}

func (p rpcPluginClient) Run(input common.PluginInput, output *common.PluginOutput) error {
	return p.Client.Call("RPCRunner.Run", input, output)
}

func (p rpcPluginClient) RunAsync(input common.PluginInput, output *common.PluginOutput, done chan *rpc.Call) *rpc.Call {
	return p.Client.Go("RPCRunner.Run", input, output, done)
}

func (p rpcPluginClient) Stop() error {
	var resp interface{}
	return p.Client.Call("RPCRunner.Stop", nil, &resp)
}

type rpcPluginTask struct {
	pluginTask

	started   bool
	client    *rpc.Client
	waitGroup sync.WaitGroup
	done      chan *rpc.Call
}

func (t *rpcPluginTask) Start() error {
	if t.started {
		return errors.New("task already started")
	}

	command := t.plugin.getExecCommand(t.operation)
	if len(command) == 0 {
		return fmt.Errorf("empty exec value")
	}

	pluginErrReader, pluginErrWriter := io.Pipe()

	var err error
	t.client, err = startRPCPlugin(pluginErrWriter, command[0], command[1:]...)
	if err != nil {
		return err
	}

	go t.handlePluginStderr(t.plugin.Name, pluginErrReader)

	iface := rpcPluginClient{
		Client: t.client,
	}

	t.done = make(chan *rpc.Call, 1)
	result := common.PluginOutput{}
	t.waitGroup.Add(1)
	iface.RunAsync(t.input, &result, t.done)
	go t.waitToFinish(&result)

	t.started = true
	return nil
}

func (t *rpcPluginTask) waitToFinish(result *common.PluginOutput) {
	defer t.client.Close()
	defer t.waitGroup.Done()
	<-t.done

	t.result = result
}

func (t *rpcPluginTask) Wait() {
	t.waitGroup.Wait()
}

func (t *rpcPluginTask) Stop() error {
	iface := rpcPluginClient{
		Client: t.client,
	}

	return iface.Stop()
}
