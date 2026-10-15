//go:build windows

package session

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/UserExistsError/conpty"
)

// winPTY wraps ConPTY.
//
// ConPTY is not a file descriptor. It is a pseudo console object that owns the
// process it launched, which is why `Wait` here is a method on the console
// rather than on an exec.Cmd. Embedding gives us Read/Write/Close/Resize/Pid
// straight from the library; only Wait needs reshaping to the shared interface.
type winPTY struct {
	*conpty.ConPty
}

func startPTY(spec ptySpec, cols, rows int) (ptyProcess, error) {
	if !conPTYAvailable() {
		// Honest failure beats a shell that looks like it works but renders
		// nothing: ConPTY needs Windows 10 1809 or newer.
		return nil, errors.New("ConPTY is not available on this system (Windows 10 1809+ required)")
	}
	opts := []conpty.ConPtyOption{conpty.ConPtyDimensions(cols, rows)}
	if spec.dir != "" {
		opts = append(opts, conpty.ConPtyWorkDir(spec.dir))
	}
	env := spec.env
	if len(env) == 0 {
		env = os.Environ()
	}
	opts = append(opts, conpty.ConPtyEnv(env))

	p, err := conpty.Start(commandLine(spec.path, spec.args), opts...)
	if err != nil {
		return nil, err
	}
	return &winPTY{ConPty: p}, nil
}

// conPTYAvailable reports whether this Windows build has a pseudo console. It
// is a property of the OS, not of this process, so a build can compile fine and
// still be unable to open a terminal.
func conPTYAvailable() bool { return conpty.IsConPtyAvailable() }

func (p *winPTY) PID() int { return p.ConPty.Pid() }

func (p *winPTY) Wait() (int, error) {
	code, err := p.ConPty.Wait(context.Background())
	if err != nil {
		return -1, err
	}
	return int(code), nil
}

// killProcess terminates the console's process tree. Closing the pseudo
// console already asks attached processes to exit; this covers the case where
// one ignores it.
func killProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

func shellCommand(shell string) (string, []string) {
	if shell != "" {
		return shell, nil
	}
	return pickWindowsShell(exec.LookPath, os.Getenv("COMSPEC")), nil
}

// commandLine builds a single command line string, because CreateProcessW takes
// a string rather than an argv. Quoting follows the rules CommandLineToArgvW
// parses back, so a path with spaces survives the round trip.
func commandLine(path string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteArg(path))
	for _, a := range args {
		parts = append(parts, quoteArg(a))
	}
	return strings.Join(parts, " ")
}

func quoteArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if !strings.ContainsAny(arg, " \t\n\v\"") {
		return arg
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range arg {
		switch r {
		case '\\':
			backslashes++
		case '"':
			// Double the preceding backslashes, then escape the quote itself.
			b.WriteString(strings.Repeat(`\`, backslashes*2+1))
			backslashes = 0
			b.WriteByte('"')
		default:
			if backslashes > 0 {
				b.WriteString(strings.Repeat(`\`, backslashes))
				backslashes = 0
			}
			b.WriteRune(r)
		}
	}
	// Trailing backslashes must be doubled so they do not escape the closing quote.
	if backslashes > 0 {
		b.WriteString(strings.Repeat(`\`, backslashes*2))
	}
	b.WriteByte('"')
	return b.String()
}
