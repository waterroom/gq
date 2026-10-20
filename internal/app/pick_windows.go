//go:build windows

package app

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	procGetOpenFileNameW     = comdlg32.NewProc("GetOpenFileNameW")
	procSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	procCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
)

const (
	ofnReadOnly      = 0x00000001
	ofnNoChangeDir   = 0x00000008
	ofnAllowMultiSel = 0x00000200
	ofnPathMustExist = 0x00000800
	ofnFileMustExist = 0x00001000
	ofnExplorer      = 0x00080000
)

// ofnW mirrors OPENFILENAMEW exactly. Field order and the implicit padding
// after each DWORD are what make this layout match the C struct on amd64.
type ofnW struct {
	StructSize      uint32
	Owner           uintptr
	Instance        uintptr
	Filter          *uint16
	CustomFilter    *uint16
	MaxCustomFilter uint32
	FilterIndex     uint32
	File            *uint16
	MaxFile         uint32
	FileTitle       *uint16
	MaxFileTitle    uint32
	InitialDir      *uint16
	Title           *uint16
	Flags           uint32
	FileOffset      uint16
	FileExtension   uint16
	DefExt          *uint16
	CustData        uintptr
	Hook            uintptr
	TemplateName    *uint16
	ReservedPtr     uintptr
	ReservedInt     uint32
	FlagsEx         uint32
}

type browseInfoW struct {
	Owner       uintptr
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	Callback    uintptr
	LParam      uintptr
	Image       int32
}

func utf16z(s string) []uint16 {
	r, _ := syscall.UTF16FromString(s)
	return r
}

func utf16p(s string) *uint16 {
	if s == "" {
		return nil
	}
	r, _ := syscall.UTF16FromString(s)
	return &r[0]
}

// pickFiles opens the classic multi-select open dialog.
func pickFiles() ([]string, error) {
	buf := make([]uint16, 32768)

	filter := append([]uint16{}, utf16z("所有文件 (*.*)")...)
	filter = append(filter, utf16z("*.*")...)
	filter = append(filter, 0)

	ofn := ofnW{
		StructSize:  uint32(unsafe.Sizeof(ofnW{})),
		Filter:      &filter[0],
		FilterIndex: 1,
		File:        &buf[0],
		MaxFile:     uint32(len(buf)),
		Title:       utf16p("选择要发送的文件"),
		Flags:       ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnAllowMultiSel | ofnNoChangeDir | ofnReadOnly,
	}

	ret, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		return nil, nil // user cancelled
	}

	// Multi-select returns "directory\0file1\0file2\0\0".
	var parts []string
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] != 0 {
			continue
		}
		if i == start {
			break
		}
		parts = append(parts, syscall.UTF16ToString(buf[start:i]))
		start = i + 1
	}
	if len(parts) == 0 {
		return nil, nil
	}
	if len(parts) == 1 {
		return []string{parts[0]}, nil
	}
	dir := parts[0]
	out := make([]string, 0, len(parts)-1)
	for _, n := range parts[1:] {
		out = append(out, filepath.Join(dir, n))
	}
	return out, nil
}

const (
	bifReturnOnlyFSDirs = 0x0001
	bifNewDialogStyle   = 0x0040
	bifEditBox          = 0x0010
)

// pickDir uses the shell folder browser. The Win32 folder dialog is ancient
// but it is one syscall away and has no COM/IUnknown plumbing.
func pickDir() (string, error) {
	bi := browseInfoW{
		Title: utf16p("选择接收目录"),
		Flags: bifReturnOnlyFSDirs | bifNewDialogStyle | bifEditBox,
	}
	pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", nil
	}
	defer procCoTaskMemFree.Call(pidl)

	buf := make([]uint16, 512)
	ok, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&buf[0])))
	if ok == 0 {
		return "", nil
	}
	return syscall.UTF16ToString(buf), nil
}
