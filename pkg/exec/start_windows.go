//go:build windows
// +build windows

package exec

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/stashapp/stash/pkg/logger"
)

// Every process started through this package is put in one job object that
// ends its processes when the job's last handle closes. Stash holds the only
// handle and never closes it, so Windows closes it when Stash exits, however
// it exits, and ends the helpers and anything they started. Processes a
// helper starts are in the job too, because job membership is inherited.
// Breaking away from the job is not allowed.
var (
	jobOnce   sync.Once
	jobHandle windows.Handle
	jobErr    error
)

func killOnExitJob() (windows.Handle, error) {
	jobOnce.Do(func() {
		// No security attributes, so the handle is not inheritable. A child
		// holding the handle would keep the job open after Stash exits.
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			jobErr = fmt.Errorf("creating job object: %w", err)
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			_ = windows.CloseHandle(h)
			jobErr = fmt.Errorf("setting job object limits: %w", err)
			return
		}
		jobHandle = h
	})
	return jobHandle, jobErr
}

func stopAll() {
	job, err := killOnExitJob()
	if err != nil {
		return // no job, so nothing was put in one
	}
	// The job stays usable: processes started later are added to it as before.
	if err := windows.TerminateJobObject(job, 1); err != nil {
		logger.Warnf("could not stop helper processes: %v", err)
	}
}

func assignToJob(p windows.Handle) error {
	job, err := killOnExitJob()
	if err != nil {
		return err
	}
	if err := windows.AssignProcessToJobObject(job, p); err != nil {
		return fmt.Errorf("assigning process to job: %w", err)
	}
	return nil
}

var procNtResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

// resumeProcess resumes every thread of a process that was created suspended.
// os/exec does not keep the main thread's handle, and finding the threads
// with a Toolhelp snapshot takes tens of milliseconds on a busy system, so
// ntdll's NtResumeProcess is used. It is not in the documented Win32 API, but
// it has been in every version of Windows NT since XP.
func resumeProcess(p windows.Handle) error {
	if err := procNtResumeProcess.Find(); err != nil {
		return err
	}
	status, _, _ := procNtResumeProcess.Call(uintptr(p))
	if status != 0 {
		return fmt.Errorf("NtResumeProcess failed: NTSTATUS %#x", status)
	}
	return nil
}

func start(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	flags := cmd.SysProcAttr.CreationFlags
	// Start suspended so the process can't start anything of its own before
	// it is in the job.
	cmd.SysProcAttr.CreationFlags = flags | windows.CREATE_SUSPENDED
	err := cmd.Start()
	cmd.SysProcAttr.CreationFlags = flags
	if err != nil {
		return err
	}

	if err := assignAndResume(cmd.Path, uint32(cmd.Process.Pid)); err != nil {
		// The process can't run. Stop it, so that Wait reports the failure
		// rather than waiting for ever.
		logger.Warnf("could not start %s: %v", cmd.Path, err)
		_ = cmd.Process.Kill()
	}
	return nil
}

func assignAndResume(name string, pid uint32) error {
	p, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, pid)
	if err != nil {
		return fmt.Errorf("opening process: %w", err)
	}
	defer windows.CloseHandle(p) //nolint:errcheck

	if err := assignToJob(p); err != nil {
		logger.Debugf("%s will not be stopped when Stash exits: %v", name, err)
	}
	return resumeProcess(p)
}

func run(cmd *exec.Cmd) error {
	if err := start(cmd); err != nil {
		return err
	}
	return cmd.Wait()
}

// output and combinedOutput follow exec.Cmd's Output and CombinedOutput,
// using start in place of Start.

func output(cmd *exec.Cmd) ([]byte, error) {
	if cmd.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	var stderr *headTailBuffer
	if cmd.Stderr == nil {
		// Like exec.Cmd.Output, keep the start and end of stderr for
		// ExitError.Stderr.
		stderr = &headTailBuffer{n: 32 << 10}
		cmd.Stderr = stderr
	}

	err := run(cmd)
	var exitErr *exec.ExitError
	if err != nil && stderr != nil && errors.As(err, &exitErr) {
		exitErr.Stderr = stderr.Bytes()
	}
	return stdout.Bytes(), err
}

func combinedOutput(cmd *exec.Cmd) ([]byte, error) {
	if cmd.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	if cmd.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}
	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = &b
	err := run(cmd)
	return b.Bytes(), err
}

// headTailBuffer keeps the first n and the last n bytes written to it.
type headTailBuffer struct {
	n       int
	head    []byte
	tail    []byte // the last bytes written after head filled, at most n
	dropped int64
}

func (b *headTailBuffer) Write(p []byte) (int, error) {
	written := len(p)
	if room := b.n - len(b.head); room > 0 {
		k := min(room, len(p))
		b.head = append(b.head, p[:k]...)
		p = p[k:]
	}
	b.tail = append(b.tail, p...)
	if over := len(b.tail) - b.n; over > 0 {
		b.dropped += int64(over)
		b.tail = append(b.tail[:0], b.tail[over:]...)
	}
	return written, nil
}

func (b *headTailBuffer) Bytes() []byte {
	if b.dropped == 0 {
		return append(append([]byte{}, b.head...), b.tail...)
	}
	var out bytes.Buffer
	out.Write(b.head)
	fmt.Fprintf(&out, "\n... omitting %d bytes ...\n", b.dropped)
	out.Write(b.tail)
	return out.Bytes()
}
