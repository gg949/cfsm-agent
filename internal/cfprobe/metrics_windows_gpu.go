//go:build windows

package cfprobe

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	windowsDXGISOK                 = uintptr(0x00000000)
	windowsDXGIEPointer            = uintptr(0x80004003)
	windowsDXGIErrorNotFound       = uintptr(0x887A0002)
	windowsDXGIAdapterFlagSoftware = uint32(1 << 1)
)

var windowsIIDIDXGIFactory1 = windowsGUID{
	Data1: 0x770AAE78,
	Data2: 0xF26F,
	Data3: 0x4DBA,
	Data4: [8]byte{0xA8, 0x29, 0x25, 0x3C, 0x83, 0xD1, 0xB3, 0x87},
}

type windowsGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type windowsLUID struct {
	LowPart  uint32
	HighPart int32
}

type windowsDXGIAdapterDesc1 struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	AdapterLUID           windowsLUID
	Flags                 uint32
}

func (d *windowsDXGIAdapterDesc1) descriptionString() string {
	end := len(d.Description)
	for i, v := range d.Description {
		if v == 0 {
			end = i
			break
		}
	}
	return strings.TrimSpace(string(utf16.Decode(d.Description[:end])))
}

type windowsDXGIFactory1 struct {
	Vtbl *windowsDXGIFactory1Vtbl
}

type windowsDXGIFactory1Vtbl struct {
	QueryInterface          uintptr
	AddRef                  uintptr
	Release                 uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr
	EnumAdapters            uintptr
	MakeWindowAssociation   uintptr
	GetWindowAssociation    uintptr
	CreateSwapChain         uintptr
	CreateSoftwareAdapter   uintptr
	EnumAdapters1           uintptr
	IsCurrent               uintptr
}

type windowsDXGIAdapter1 struct {
	Vtbl *windowsDXGIAdapter1Vtbl
}

type windowsDXGIAdapter1Vtbl struct {
	QueryInterface          uintptr
	AddRef                  uintptr
	Release                 uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr
	EnumOutputs             uintptr
	GetDesc                 uintptr
	CheckInterfaceSupport   uintptr
	GetDesc1                uintptr
}

type windowsGPUAdapter struct {
	Name         string
	LUIDHighPart int32
	LUIDLowPart  uint32
	Flags        uint32
}

func (f *windowsDXGIFactory1) enumAdapters1(adapterIndex uint32, adapter **windowsDXGIAdapter1) uintptr {
	if f == nil || f.Vtbl == nil || f.Vtbl.EnumAdapters1 == 0 || adapter == nil {
		return windowsDXGIEPointer
	}
	ret, _, _ := syscall.SyscallN(
		f.Vtbl.EnumAdapters1,
		uintptr(unsafe.Pointer(f)),
		uintptr(adapterIndex),
		uintptr(unsafe.Pointer(adapter)),
	)
	return ret
}

func (f *windowsDXGIFactory1) release() uint32 {
	if f == nil || f.Vtbl == nil || f.Vtbl.Release == 0 {
		return 0
	}
	ret, _, _ := syscall.SyscallN(f.Vtbl.Release, uintptr(unsafe.Pointer(f)))
	return uint32(ret)
}

func (a *windowsDXGIAdapter1) getDesc1(desc *windowsDXGIAdapterDesc1) uintptr {
	if a == nil || a.Vtbl == nil || a.Vtbl.GetDesc1 == 0 || desc == nil {
		return windowsDXGIEPointer
	}
	ret, _, _ := syscall.SyscallN(
		a.Vtbl.GetDesc1,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(desc)),
	)
	return ret
}

func (a *windowsDXGIAdapter1) release() uint32 {
	if a == nil || a.Vtbl == nil || a.Vtbl.Release == 0 {
		return 0
	}
	ret, _, _ := syscall.SyscallN(a.Vtbl.Release, uintptr(unsafe.Pointer(a)))
	return uint32(ret)
}

func windowsGPUInfo() any {
	if gpu := detectGPUInfo(); gpu != nil {
		return gpu
	}
	adapters, err := windowsDXGIAdapters()
	if err != nil {
		return nil
	}
	type gpuKey struct {
		LUIDHighPart int32
		LUIDLowPart  uint32
	}
	seen := map[gpuKey]bool{}
	var gpus []gpuMetric
	for _, adapter := range adapters {
		if adapter.Flags&windowsDXGIAdapterFlagSoftware != 0 || adapter.Name == "" {
			continue
		}
		key := gpuKey{LUIDHighPart: adapter.LUIDHighPart, LUIDLowPart: adapter.LUIDLowPart}
		if seen[key] {
			continue
		}
		seen[key] = true
		gpus = append(gpus, gpuMetric{Name: adapter.Name, Info: 0, ID: strconv.Itoa(len(gpus))})
	}
	if len(gpus) == 0 {
		return nil
	}
	return gpus
}

func windowsDXGIAdapters() ([]windowsGPUAdapter, error) {
	dxgiDLL, err := syscall.LoadDLL("dxgi.dll")
	if err != nil {
		return nil, fmt.Errorf("load dxgi.dll: %w", err)
	}
	defer dxgiDLL.Release()

	createDXGIFactory1, err := dxgiDLL.FindProc("CreateDXGIFactory1")
	if err != nil {
		return nil, fmt.Errorf("find CreateDXGIFactory1: %w", err)
	}

	var factory *windowsDXGIFactory1
	ret, _, _ := syscall.SyscallN(
		createDXGIFactory1.Addr(),
		uintptr(unsafe.Pointer(&windowsIIDIDXGIFactory1)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if ret != windowsDXGISOK {
		if factory != nil {
			factory.release()
		}
		return nil, fmt.Errorf("CreateDXGIFactory1 HRESULT 0x%08X", uint32(ret))
	}
	if factory == nil {
		return nil, fmt.Errorf("CreateDXGIFactory1 returned nil factory")
	}
	defer factory.release()

	var adapters []windowsGPUAdapter
	for adapterIndex := uint32(0); ; adapterIndex++ {
		var adapter *windowsDXGIAdapter1
		hr := factory.enumAdapters1(adapterIndex, &adapter)
		if hr == windowsDXGIErrorNotFound {
			break
		}
		if hr != windowsDXGISOK {
			if adapter != nil {
				adapter.release()
			}
			return adapters, fmt.Errorf("EnumAdapters1 index %d HRESULT 0x%08X", adapterIndex, uint32(hr))
		}
		if adapter == nil {
			return adapters, fmt.Errorf("EnumAdapters1 returned nil adapter at index %d", adapterIndex)
		}

		var desc windowsDXGIAdapterDesc1
		hr = adapter.getDesc1(&desc)
		adapter.release()
		if hr != windowsDXGISOK {
			return adapters, fmt.Errorf("GetDesc1 index %d HRESULT 0x%08X", adapterIndex, uint32(hr))
		}
		adapters = append(adapters, windowsGPUAdapter{
			Name:         desc.descriptionString(),
			LUIDHighPart: desc.AdapterLUID.HighPart,
			LUIDLowPart:  desc.AdapterLUID.LowPart,
			Flags:        desc.Flags,
		})
	}
	return adapters, nil
}
