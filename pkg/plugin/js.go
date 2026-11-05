package plugin

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/dop251/goja"
	"github.com/stashapp/stash/pkg/javascript"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/plugin/common"
)

var errStop = errors.New("stop")

type jsTaskBuilder struct{}

func (*jsTaskBuilder) build(task pluginTask) Task {
	return &jsPluginTask{
		pluginTask: task,
	}
}

type jsPluginTask struct {
	pluginTask

	started   bool
	waitGroup sync.WaitGroup
	vm        *javascript.VM
}

func (t *jsPluginTask) onError(err error) {
	errString := err.Error()
	t.result = &common.PluginOutput{
		Error: &errString,
	}
}

func (t *jsPluginTask) makeOutput(o goja.Value) {
	t.result = &common.PluginOutput{}

	asObj := o.ToObject(t.vm.Runtime)
	if asObj == nil {
		return
	}

	t.result.Output = asObj.Get("Output")
	err := asObj.Get("Error")
	if !goja.IsNull(err) && !goja.IsUndefined(err) {
		errStr := err.String()
		t.result.Error = &errStr
	}
}

func (t *jsPluginTask) initVM() error {
	// converting the Args field to map[string]interface{} is required, otherwise
	// it gets converted to an empty object
	// ideally this should have included json tags with the correct casing but changing
	// it now will result in a breaking change
	type pluginInput struct {
		// Server details to connect to the stash server.
		ServerConnection common.StashServerConnection

		// Arguments to the plugin operation.
		Args map[string]interface{}

		// Configured settings for the plugin.
		Settings map[string]interface{}
	}

	input := pluginInput{
		ServerConnection: t.input.ServerConnection,
		Args:             t.input.Args.ToMap(),
		Settings:         t.input.Settings,
	}

	if err := t.vm.Set("input", input); err != nil {
		return fmt.Errorf("error setting input: %w", err)
	}

	const pluginPrefix = "[Plugin / %s] "

	log := &javascript.Log{
		Logger:       logger.Logger,
		Prefix:       fmt.Sprintf(pluginPrefix, t.plugin.Name),
		ProgressChan: t.progress,
	}

	if err := log.AddToVM("log", t.vm); err != nil {
		return fmt.Errorf("error adding log API: %w", err)
	}

	util := &javascript.Util{}
	if err := util.AddToVM("util", t.vm); err != nil {
		return fmt.Errorf("error adding util API: %w", err)
	}

	// The plugin's GraphQL calls skip the login check, so they carry the user
	// whose request started the plugin, as an external plugin's session cookie
	// does. Only the user and what the cookie would carry are taken: the
	// request itself may be finished, or cancelled, or in a transaction by
	// the time the plugin runs. The login time and API key fingerprint let
	// each call be checked again, and go into cookies for hooks it starts.
	gqlCtx := t.starter.PluginContext()

	gql := &javascript.GQL{
		Context:    gqlCtx,
		Cookie:     t.input.ServerConnection.SessionCookie,
		GQLHandler: t.gqlHandler,
	}
	if err := gql.AddToVM("gql", t.vm); err != nil {
		return fmt.Errorf("error adding GraphQL API: %w", err)
	}

	return nil
}

func (t *jsPluginTask) Start() error {
	if t.started {
		return errors.New("task already started")
	}

	t.started = true

	if len(t.plugin.Exec) == 0 {
		return errors.New("no script specified in exec")
	}

	scriptFile := t.plugin.Exec[0]

	t.vm = javascript.NewVM()
	pluginPath := t.plugin.getConfigPath()
	script, err := javascript.Compile(filepath.Join(pluginPath, scriptFile))
	if err != nil {
		return err
	}

	if err := t.initVM(); err != nil {
		return err
	}

	t.waitGroup.Add(1)

	go func() {
		defer func() {
			t.waitGroup.Done()

			if caught := recover(); caught != nil {
				if err, ok := caught.(error); ok && errors.Is(err, errStop) {
					// TODO - log this
					return
				}
			}
		}()

		output, err := t.vm.RunProgram(script)

		if err != nil {
			t.onError(err)
		} else {
			t.makeOutput(output)
		}
	}()

	return nil
}

func (t *jsPluginTask) Wait() {
	t.waitGroup.Wait()
}

func (t *jsPluginTask) Stop() error {
	t.vm.Interrupt(errStop)
	return nil
}
