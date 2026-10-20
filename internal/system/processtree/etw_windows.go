// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

//go:build windows && (amd64 || arm64)

package processtree

import (
	"encoding/binary"
	"errors"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The event feed is a private real-time ETW session subscribed to
// Microsoft-Windows-Kernel-Process. It reports every process start with its
// parent and image, and every exit, as they happen, so a launcher that lives
// for 50 ms still leaves its link in the chain. Creating the session needs
// administrator rights; without them the tracker polls instead.

var (
	modadvapi32 = windows.NewLazySystemDLL("advapi32.dll")
	modtdh      = windows.NewLazySystemDLL("tdh.dll")

	procStartTraceW        = modadvapi32.NewProc("StartTraceW")
	procControlTraceW      = modadvapi32.NewProc("ControlTraceW")
	procEnableTraceEx2     = modadvapi32.NewProc("EnableTraceEx2")
	procOpenTraceW         = modadvapi32.NewProc("OpenTraceW")
	procProcessTrace       = modadvapi32.NewProc("ProcessTrace")
	procCloseTrace         = modadvapi32.NewProc("CloseTrace")
	procTdhGetPropertySize = modtdh.NewProc("TdhGetPropertySize")
	procTdhGetProperty     = modtdh.NewProc("TdhGetProperty")
)

var kernelProcessProvider = windows.GUID{
	Data1: 0x22fb2cd6, Data2: 0x0e7b, Data3: 0x422b,
	Data4: [8]byte{0xa0, 0xc7, 0x2f, 0xad, 0x1f, 0xd0, 0xe7, 0x16},
}

const (
	etwSessionName = "ResultV Process Lineage"

	wnodeFlagTracedGUID         = 0x00020000
	eventTraceRealTimeMode      = 0x00000100
	eventTraceUseMsFlushTimer   = 0x00000010
	eventTraceControlStop       = 1
	eventControlCodeEnable      = 1
	traceLevelInformation       = 4
	keywordProcess              = 0x10
	processTraceModeRealTime    = 0x00000100
	processTraceModeEventRecord = 0x10000000
	invalidProcessTraceHandle   = ^uint64(0)

	eventProcessStart = 1
	eventProcessStop  = 2

	errorAlreadyExists     = 183
	errorInvalidParameter  = 87
	propertyArrayIndexNone = 0xFFFFFFFF
)

type wnodeHeader struct {
	BufferSize        uint32
	ProviderID        uint32
	HistoricalContext uint64
	TimeStamp         int64
	GUID              windows.GUID
	ClientContext     uint32
	Flags             uint32
}

type eventTraceProperties struct {
	Wnode               wnodeHeader
	BufferSize          uint32
	MinimumBuffers      uint32
	MaximumBuffers      uint32
	MaximumFileSize     uint32
	LogFileMode         uint32
	FlushTimer          uint32
	EnableFlags         uint32
	AgeLimit            int32
	NumberOfBuffers     uint32
	FreeBuffers         uint32
	EventsLost          uint32
	BuffersWritten      uint32
	LogBuffersLost      uint32
	RealTimeBuffersLost uint32
	LoggerThreadID      uintptr
	LogFileNameOffset   uint32
	LoggerNameOffset    uint32
}

// propertiesBuffer is EVENT_TRACE_PROPERTIES with room behind it for the
// session name, which the API writes at LoggerNameOffset.
type propertiesBuffer struct {
	props eventTraceProperties
	name  [256]uint16
}

func newProperties(logFileMode, flushTimer uint32) *propertiesBuffer {
	b := &propertiesBuffer{}
	b.props.Wnode.BufferSize = uint32(unsafe.Sizeof(*b))
	b.props.Wnode.Flags = wnodeFlagTracedGUID
	b.props.Wnode.ClientContext = 1
	b.props.LogFileMode = logFileMode
	b.props.FlushTimer = flushTimer
	b.props.BufferSize = 16
	b.props.MinimumBuffers = 4
	b.props.MaximumBuffers = 16
	b.props.LoggerNameOffset = uint32(unsafe.Offsetof(b.name))
	return b
}

// eventTraceLogfile is EVENT_TRACE_LOGFILEW. The two embedded structs nothing
// here reads are kept as opaque bytes of their 64-bit size.
type eventTraceLogfile struct {
	LogFileName         *uint16
	LoggerName          *uint16
	CurrentTime         int64
	BuffersRead         uint32
	ProcessTraceMode    uint32
	CurrentEvent        [88]byte
	LogfileHeader       [280]byte
	BufferCallback      uintptr
	BufferSize          uint32
	Filled              uint32
	EventsLost          uint32
	EventRecordCallback uintptr
	IsKernelTrace       uint32
	Context             uintptr
}

type eventDescriptor struct {
	ID      uint16
	Version uint8
	Channel uint8
	Level   uint8
	Opcode  uint8
	Task    uint16
	Keyword uint64
}

type eventHeader struct {
	Size            uint16
	HeaderType      uint16
	Flags           uint16
	EventProperty   uint16
	ThreadID        uint32
	ProcessID       uint32
	TimeStamp       int64
	ProviderID      windows.GUID
	EventDescriptor eventDescriptor
	ProcessorTime   uint64
	ActivityID      windows.GUID
}

type eventRecord struct {
	EventHeader       eventHeader
	BufferContext     uint32
	ExtendedDataCount uint16
	UserDataLength    uint16
	ExtendedData      uintptr
	UserData          uintptr
	UserContext       uintptr
}

type propertyDataDescriptor struct {
	PropertyName uint64
	ArrayIndex   uint32
	Reserved     uint32
}

var (
	propProcessID       = mustUTF16("ProcessID")
	propParentProcessID = mustUTF16("ParentProcessID")
	propCreateTime      = mustUTF16("CreateTime")
	propExitTime        = mustUTF16("ExitTime")
	propImageName       = mustUTF16("ImageName")
)

func mustUTF16(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		panic(err)
	}
	return p
}

type eventFeed struct {
	session uint64
	trace   uint64
	logfile *eventTraceLogfile
	done    chan struct{}
}

var (
	feedMu      sync.Mutex
	feed        *eventFeed
	feedTracker *Tracker

	callbackOnce sync.Once
	callbackPtr  uintptr
)

func startEventFeed(t *Tracker) error {
	if err := modadvapi32.Load(); err != nil {
		return err
	}
	if err := modtdh.Load(); err != nil {
		return err
	}
	name := mustUTF16(etwSessionName)

	// A session left behind by a crashed run keeps the name taken.
	stopSessionByName(name)

	var session uint64
	err := startTrace(&session, name, eventTraceRealTimeMode|eventTraceUseMsFlushTimer, 100)
	if errno, ok := err.(syscall.Errno); ok && errno == errorInvalidParameter {
		err = startTrace(&session, name, eventTraceRealTimeMode, 1)
	}
	if err != nil {
		return err
	}

	guid := kernelProcessProvider
	if r, _, _ := procEnableTraceEx2.Call(
		uintptr(session),
		uintptr(unsafe.Pointer(&guid)),
		eventControlCodeEnable,
		traceLevelInformation,
		keywordProcess,
		0, 0, 0,
	); r != 0 {
		stopSession(session)
		return syscall.Errno(r)
	}

	callbackOnce.Do(func() { callbackPtr = syscall.NewCallback(onEvent) })
	feedMu.Lock()
	feedTracker = t
	feedMu.Unlock()

	logfile := &eventTraceLogfile{
		LoggerName:          name,
		ProcessTraceMode:    processTraceModeRealTime | processTraceModeEventRecord,
		EventRecordCallback: callbackPtr,
	}
	r, _, _ := procOpenTraceW.Call(uintptr(unsafe.Pointer(logfile)))
	trace := uint64(r)
	if trace == invalidProcessTraceHandle {
		stopSession(session)
		return errors.New("processtree: OpenTrace failed")
	}

	f := &eventFeed{session: session, trace: trace, logfile: logfile, done: make(chan struct{})}
	feedMu.Lock()
	feed = f
	feedMu.Unlock()

	go func() {
		// ProcessTrace delivers every event on the thread that called it.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		handle := trace
		procProcessTrace.Call(uintptr(unsafe.Pointer(&handle)), 1, 0, 0)
		close(f.done)
	}()
	return nil
}

func startTrace(session *uint64, name *uint16, mode, flush uint32) error {
	props := newProperties(mode, flush)
	r, _, _ := procStartTraceW.Call(
		uintptr(unsafe.Pointer(session)),
		uintptr(unsafe.Pointer(name)),
		uintptr(unsafe.Pointer(props)),
	)
	if r == errorAlreadyExists {
		stopSessionByName(name)
		r, _, _ = procStartTraceW.Call(
			uintptr(unsafe.Pointer(session)),
			uintptr(unsafe.Pointer(name)),
			uintptr(unsafe.Pointer(props)),
		)
	}
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func stopSessionByName(name *uint16) {
	props := newProperties(0, 0)
	procControlTraceW.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(props)), eventTraceControlStop)
}

func stopSession(session uint64) {
	props := newProperties(0, 0)
	procControlTraceW.Call(uintptr(session), 0, uintptr(unsafe.Pointer(props)), eventTraceControlStop)
}

func stopEventFeed() {
	feedMu.Lock()
	f := feed
	feed = nil
	feedMu.Unlock()
	if f == nil {
		return
	}
	stopSession(f.session)
	<-f.done
	procCloseTrace.Call(uintptr(f.trace))
}

func onEvent(rec *eventRecord) uintptr {
	if rec == nil || rec.EventHeader.ProviderID != kernelProcessProvider {
		return 0
	}
	feedMu.Lock()
	t := feedTracker
	feedMu.Unlock()
	if t == nil {
		return 0
	}
	switch rec.EventHeader.EventDescriptor.ID {
	case eventProcessStart:
		pid, ok1 := propertyUint32(rec, propProcessID)
		ppid, ok2 := propertyUint32(rec, propParentProcessID)
		created, ok3 := propertyInt64(rec, propCreateTime)
		if !ok1 || !ok2 || !ok3 {
			return 0
		}
		t.Started(Process{PID: pid, ParentPID: ppid, Created: created, Image: dosPath(propertyString(rec, propImageName))})
	case eventProcessStop:
		pid, ok1 := propertyUint32(rec, propProcessID)
		created, ok2 := propertyInt64(rec, propCreateTime)
		exited, ok3 := propertyInt64(rec, propExitTime)
		if !ok1 || !ok2 || !ok3 {
			return 0
		}
		t.Exited(pid, created, exited)
	}
	return 0
}

func property(rec *eventRecord, name *uint16) []byte {
	desc := propertyDataDescriptor{
		PropertyName: uint64(uintptr(unsafe.Pointer(name))),
		ArrayIndex:   propertyArrayIndexNone,
	}
	var size uint32
	if r, _, _ := procTdhGetPropertySize.Call(
		uintptr(unsafe.Pointer(rec)), 0, 0, 1,
		uintptr(unsafe.Pointer(&desc)),
		uintptr(unsafe.Pointer(&size)),
	); r != 0 || size == 0 {
		return nil
	}
	buf := make([]byte, size)
	if r, _, _ := procTdhGetProperty.Call(
		uintptr(unsafe.Pointer(rec)), 0, 0, 1,
		uintptr(unsafe.Pointer(&desc)),
		uintptr(size),
		uintptr(unsafe.Pointer(&buf[0])),
	); r != 0 {
		return nil
	}
	return buf
}

func propertyUint32(rec *eventRecord, name *uint16) (uint32, bool) {
	b := property(rec, name)
	if len(b) < 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b), true
}

func propertyInt64(rec *eventRecord, name *uint16) (int64, bool) {
	b := property(rec, name)
	if len(b) < 8 {
		return 0, false
	}
	return int64(binary.LittleEndian.Uint64(b)), true
}

func propertyString(rec *eventRecord, name *uint16) string {
	b := property(rec, name)
	if len(b) < 2 {
		return ""
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return windows.UTF16ToString(u)
}

var (
	dosDevicesMu sync.Mutex
	dosDevices   map[string]string
	dosDevicesAt time.Time
)

// dosPath turns the NT device path an event reports into the drive-letter
// path a live query gives, so a list entry written as a full path matches
// both. A path on no mounted drive is returned as it came.
func dosPath(p string) string {
	if !strings.HasPrefix(p, `\Device\`) {
		return p
	}
	dosDevicesMu.Lock()
	defer dosDevicesMu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if dosDevices == nil || attempt == 1 && time.Since(dosDevicesAt) > time.Minute {
			dosDevices = readDosDevices()
			dosDevicesAt = time.Now()
		}
		for device, drive := range dosDevices {
			if strings.HasPrefix(p, device+`\`) {
				return drive + p[len(device):]
			}
		}
	}
	return p
}

func readDosDevices() map[string]string {
	out := make(map[string]string)
	buf := make([]uint16, 1024)
	for c := 'A'; c <= 'Z'; c++ {
		drive := string(c) + ":"
		name, _ := windows.UTF16PtrFromString(drive)
		if n, err := windows.QueryDosDevice(name, &buf[0], uint32(len(buf))); err == nil && n > 0 {
			out[windows.UTF16ToString(buf[:n])] = drive
		}
	}
	return out
}
