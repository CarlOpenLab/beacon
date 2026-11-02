//go:build windows

package session

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// liveCwd is where a process stands on Windows: a shell's current directory
// lives in its process environment block, which another process of the same
// user can read. The PEB's address comes from NtQueryInformationProcess, and
// the directory itself is the RTL_USER_PROCESS_PARAMETERS.CurrentDirectory
// UNICODE_STRING — the same field GetCurrentDirectory reads for the process
// itself, so a shell that has cd'd is answered with the place it cd'd to.
//
// A ConPTY owns the process it launched, so the session's pid is the shell's
// pid and the field is the shell's own. Anything that cannot be read — a
// protected process, a bitness this build cannot address, a process that has
// already gone — is "", and the caller falls back to the started directory.
func liveCwd(pid int) string {
	if pid <= 0 {
		return ""
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)

	peb, layout, ok := targetPEB(h)
	if !ok {
		return ""
	}
	params, err := readPointer(h, peb+layout.processParameters, layout.pointerSize)
	if err != nil || params == 0 {
		return ""
	}
	// The UNICODE_STRING header — its byte length and the path pointer — is
	// one read: both fields sit in the same few bytes, and a second
	// cross-process read only to fetch the pointer would cost more than the
	// bytes are worth.
	head := make([]byte, int(layout.bufferOffset)+layout.pointerSize)
	if err := readProcessMemory(h, params+layout.currentDirectory, head); err != nil {
		return ""
	}
	length := binary.LittleEndian.Uint16(head[:2])
	if length == 0 || length%2 != 0 {
		return ""
	}
	buf := uintptrAt(head[layout.bufferOffset:], layout.pointerSize)
	if buf == 0 {
		return ""
	}
	// The path is UTF-16LE, and its length is in bytes. Reading the code units
	// straight into the string's backing store skips a byte slice and a copy;
	// every Windows architecture Go builds for is little-endian, like the PEB.
	units := make([]uint16, length/2)
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&units[0])), int(length))
	if err := readProcessMemory(h, buf, raw); err != nil {
		return ""
	}
	dir := trimNTNamespace(windows.UTF16ToString(units))
	if dir == "" {
		return ""
	}
	// The stored string can carry a trailing separator ("C:\work\"), which the
	// start directory never does; one form is compared and carried around.
	return filepath.Clean(dir)
}

// pebAddress asks ntdll where the process's PEB is.
func pebAddress(h windows.Handle) (uintptr, error) {
	var info windows.PROCESS_BASIC_INFORMATION
	var size uint32
	err := windows.NtQueryInformationProcess(h, windows.ProcessBasicInformation,
		unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)), &size)
	if err != nil {
		return 0, err
	}
	return uintptr(unsafe.Pointer(info.PebBaseAddress)), nil
}

// pebLayout is where the two fields this file reads sit in a target of one
// bitness: the PEB's pointer to RTL_USER_PROCESS_PARAMETERS, the
// parameters' CurrentDirectory UNICODE_STRING, and the UNICODE_STRING's path
// buffer pointer.
type pebLayout struct {
	processParameters uintptr // PEB -> RTL_USER_PROCESS_PARAMETERS
	currentDirectory  uintptr // parameters -> CURDIR.DosPath
	bufferOffset      uintptr // UNICODE_STRING -> Buffer
	pointerSize       int
}

var (
	// layoutNative is this build's own bitness, read off the structures x/sys
	// compiles for it rather than restated here.
	layoutNative = pebLayout{
		processParameters: uintptr(unsafe.Offsetof(windows.PEB{}.ProcessParameters)),
		currentDirectory: uintptr(unsafe.Offsetof(windows.RTL_USER_PROCESS_PARAMETERS{}.CurrentDirectory)) +
			uintptr(unsafe.Offsetof(windows.CURDIR{}.DosPath)),
		bufferOffset: uintptr(unsafe.Offsetof(windows.NTUnicodeString{}.Buffer)),
		pointerSize:  int(unsafe.Sizeof(uintptr(0))),
	}
	// layout32 is a 32-bit shell under WOW64: x/sys lays out this build's own
	// bitness only, so the x86 PEB is stated here.
	layout32 = pebLayout{processParameters: 0x10, currentDirectory: 0x24, bufferOffset: 4, pointerSize: 4}
)

// targetPEB is where the process behind h keeps the environment block this
// file reads, with the layout that block is in.
//
// A 32-bit shell under WOW64 has both a 64-bit and a 32-bit PEB, and its
// parameters live in the 32-bit one: ProcessBasicInformation would name the
// 64-bit PEB, whose pointer does not lead to them, so a WOW64 target is asked
// for ProcessWow64Information instead. A target that is not WOW64 is native
// to this Windows, and its block is laid out like this build's own.
func targetPEB(h windows.Handle) (uintptr, pebLayout, bool) {
	var wow64 bool
	if err := windows.IsWow64Process(h, &wow64); err != nil {
		return 0, pebLayout{}, false
	}
	if wow64 {
		var peb uintptr
		var size uint32
		err := windows.NtQueryInformationProcess(h, windows.ProcessWow64Information,
			unsafe.Pointer(&peb), uint32(unsafe.Sizeof(peb)), &size)
		if err != nil || peb == 0 {
			return 0, pebLayout{}, false
		}
		return peb, layout32, true
	}
	peb, err := pebAddress(h)
	if err != nil || peb == 0 {
		return 0, pebLayout{}, false
	}
	return peb, layoutNative, true
}

// readPointer reads a pointer-sized address from another process.
func readPointer(h windows.Handle, addr uintptr, size int) (uintptr, error) {
	var b [8]byte
	if err := readProcessMemory(h, addr, b[:size]); err != nil {
		return 0, err
	}
	return uintptrAt(b[:], size), nil
}

// uintptrAt decodes a pointer-sized little-endian value from b.
func uintptrAt(b []byte, size int) uintptr {
	if size == 4 {
		return uintptr(binary.LittleEndian.Uint32(b[:4]))
	}
	return uintptr(binary.LittleEndian.Uint64(b[:8]))
}

// readProcessMemory fills buf from another process. Windows fails the call
// outright when it cannot copy every byte (ERROR_PARTIAL_COPY), so an error
// means no part of buf is trusted and the caller has nothing to use.
func readProcessMemory(h windows.Handle, addr uintptr, buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	var read uintptr
	return windows.ReadProcessMemory(h, addr, &buf[0], uintptr(len(buf)), &read)
}

// trimNTNamespace removes the NT object-manager prefixes a stored directory
// path can carry ("\??\C:\...", "\\?\C:\...") so the answer is a path another
// process can open.
func trimNTNamespace(dir string) string {
	if strings.HasPrefix(dir, `\??\`) || strings.HasPrefix(dir, `\\?\`) {
		return dir[4:]
	}
	return dir
}
