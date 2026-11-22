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
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework SystemConfiguration
#import <Foundation/Foundation.h>
#import <SystemConfiguration/SystemConfiguration.h>
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdbool.h>
#include <sys/kauth.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/un.h>
#include <unistd.h>

static char *controlFolder(uid_t *uid, gid_t *gid) {
	@autoreleasepool {
		if (!@available(macOS 26.0, *)) { return NULL; }
		NSString *group = [[NSBundle mainBundle] objectForInfoDictionaryKey:@"ResultVControlGroup"];
		if (group == nil) { return NULL; }

		if (geteuid() != 0) {
			*uid = getuid();
			*gid = getgid();
			NSURL *url = [[NSFileManager defaultManager] containerURLForSecurityApplicationGroupIdentifier:group];
			return url ? strdup(url.fileSystemRepresentation) : NULL;
		}

		// An elevated ResultV is relaunched in the GUI user's bootstrap, with
		// access to the container created before the administrator relaunch.
		CFStringRef user = SCDynamicStoreCopyConsoleUser(NULL, uid, gid);
		if (user == NULL) { return NULL; }
		NSString *home = *uid == 0 ? nil : NSHomeDirectoryForUser((__bridge NSString *)user);
		CFRelease(user);
		if (home == nil) { return NULL; }
		NSString *base = [home stringByAppendingPathComponent:@"Library/Group Containers"];
		return strdup([[base stringByAppendingPathComponent:group] fileSystemRepresentation]);
	}
}

static int controlListenerAsUser(const char *path, uid_t uid) {
	struct sockaddr_un address = {0};
	if (strlen(path) >= sizeof(address.sun_path)) { errno = ENAMETOOLONG; return -1; }
	address.sun_family = AF_UNIX;
	address.sun_len = sizeof(address);
	strlcpy(address.sun_path, path, sizeof(address.sun_path));

	int fd = socket(AF_UNIX, SOCK_STREAM, 0);
	if (fd < 0) { return -1; }
	if (fcntl(fd, F_SETFD, FD_CLOEXEC) < 0) { goto fail; }
	if (bind(fd, (struct sockaddr*)&address, sizeof(address)) < 0) {
		if (errno != EADDRINUSE) { goto fail; }
		// Replace a stale socket only after checking its owner and type.
		struct stat info;
		if (lstat(path, &info) < 0) { goto fail; }
		if (!S_ISSOCK(info.st_mode) || info.st_uid != uid) { errno = EADDRINUSE; goto fail; }
		int probe = socket(AF_UNIX, SOCK_STREAM, 0);
		if (probe < 0) { goto fail; }
		if (fcntl(probe, F_SETFL, O_NONBLOCK) < 0) { close(probe); goto fail; }
		int result = connect(probe, (struct sockaddr*)&address, sizeof(address));
		int probeError = errno;
		close(probe);
		if (result == 0 || probeError != ECONNREFUSED) { errno = EADDRINUSE; goto fail; }
		if (unlink(path) < 0 || bind(fd, (struct sockaddr*)&address, sizeof(address)) < 0) { goto fail; }
	}
	if (listen(fd, 8) < 0) { goto fail; }
	return fd;

fail: {
	int error = errno;
	close(fd);
	errno = error;
	return -1;
}
}

static int controlListener(const char *path, uid_t uid, gid_t gid) {
	// Scope filesystem operations to the GUI user even in the elevated app.
	bool elevated = geteuid() == 0;
	if (elevated && pthread_setugid_np(uid, gid) < 0) { return -1; }
	int fd = controlListenerAsUser(path, uid);
	int error = errno;
	if (elevated && pthread_setugid_np(KAUTH_UID_NONE, KAUTH_GID_NONE) < 0) { abort(); }
	errno = error;
	return fd;
}

static int controlPeerMatches(int fd, uid_t expected) {
	uid_t uid;
	gid_t gid;
	return getpeereid(fd, &uid, &gid) == 0 && uid == expected;
}

static int reloadVPNControl(void) {
	static void (*reload)(void) = NULL;
	if (reload != NULL) { reload(); return 1; }
	@autoreleasepool {
		NSURL *bundle = [[NSBundle mainBundle] privateFrameworksURL];
		NSURL *library = [bundle URLByAppendingPathComponent:@"ResultVControlReload.dylib"];
		if (library == nil) { return 0; }
		void *handle = dlopen(library.fileSystemRepresentation, RTLD_LOCAL | RTLD_LAZY);
		if (handle == NULL) { return 0; }
		reload = dlsym(handle, "ResultVReloadVPNControl");
		if (reload == NULL) { dlclose(handle); return 0; }
		reload();
		return 1;
	}
}
*/
import "C"

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
	"unsafe"

	"resultproxy-wails/internal/proxy"
	"resultproxy-wails/internal/system"
)

func (a *App) startControlCenter() func() {
	var uid C.uid_t
	var gid C.gid_t
	folder := C.controlFolder(&uid, &gid)
	if folder == nil {
		return func() {}
	}
	defer C.free(unsafe.Pointer(folder))
	listener, err := listenControlSocket(filepath.Join(C.GoString(folder), "ctl.sock"), uint32(uid), uint32(gid))
	if err != nil {
		a.log.Warning(fmt.Sprintf("[CONTROL CENTER] Не удалось открыть сокет: %v", err))
		return func() {}
	}

	var busy atomic.Bool
	reload := make(chan struct{}, 1)
	scheduleReload := func() {
		select {
		case reload <- struct{}{}:
		default:
		}
	}
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		connected, _ := a.proxy.SessionState()
		refresh := C.reloadVPNControl() != 0
		if !refresh {
			return
		}
		for {
			select {
			case <-stop:
				C.reloadVPNControl()
				return
			case <-reload:
				connected, _ = a.proxy.SessionState()
				C.reloadVPNControl()
			case <-ticker.C:
				current, _ := a.proxy.SessionState()
				if current != connected {
					connected = current
					C.reloadVPNControl()
				}
			}
		}
	}()
	go func() {
		for {
			conn, err := listener.AcceptUnix()
			if err != nil {
				return
			}
			start := a.handleVPNControl(conn, uint32(uid), &busy)
			_ = conn.Close()
			if start != nil {
				go func() {
					defer scheduleReload()
					defer busy.Store(false)
					if err := start(); err != nil {
						a.log.Warning(fmt.Sprintf("[CONTROL CENTER] %v", err))
					}
				}()
			}
		}
	}()
	return func() {
		_ = listener.Close()
		close(stop)
	}
}

func listenControlSocket(path string, uid, gid uint32) (*net.UnixListener, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	fd, err := C.controlListener(cPath, C.uid_t(uid), C.gid_t(gid))
	if fd < 0 {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "ResultV Control Center")
	defer file.Close()
	listener, err := net.FileListener(file)
	if err != nil {
		return nil, err
	}
	return listener.(*net.UnixListener), nil
}

func (a *App) handleVPNControl(conn *net.UnixConn, uid uint32, busy *atomic.Bool) func() error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil
	}
	allowed := false
	if err := raw.Control(func(fd uintptr) {
		allowed = C.controlPeerMatches(C.int(fd), C.uid_t(uid)) != 0
	}); err != nil || !allowed {
		return nil
	}
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	var command [1]byte
	if _, err := io.ReadFull(conn, command[:]); err != nil {
		return nil
	}

	connected, establishing := a.proxy.SessionState()
	var action func() error
	reply := byte('0')
	if connected {
		reply = '1'
	}
	if command[0] != 's' {
		if busy.Load() {
			reply = 'e'
		} else {
			reply, action = a.controlCenterAction(command[0], connected, establishing)
			if action != nil {
				busy.Store(true)
			}
		}
	}
	if _, err := conn.Write([]byte{reply}); err != nil && action != nil {
		busy.Store(false)
		return nil
	}
	return action
}

func (a *App) controlCenterAction(command byte, connected, establishing bool) (byte, func() error) {
	a.stateMu.Lock()
	quitting := a.quitRequested
	a.stateMu.Unlock()
	if quitting || establishing {
		return 'e', nil
	}
	switch command {
	case '0':
		if !connected {
			return '0', nil
		}
		return '0', a.Disconnect
	case '1':
		if connected {
			return '1', nil
		}
		settings := a.config.GetConfig().Settings
		if settings.LastSelectedProxyID == "" {
			return 'e', nil
		}
		return '1', func() error {
			if settings.Mode == string(proxy.ProxyModeTunnel) && !system.IsAdmin() {
				return a.RestartAsAdmin(settings.LastSelectedProxyID, settings.Mode)
			}
			a.waitForLeftoverCleanup(15 * time.Second)
			a.stateMu.Lock()
			quitting := a.quitRequested
			a.stateMu.Unlock()
			if quitting {
				return nil
			}
			return a.connectFromTray(settings.LastSelectedProxyID)
		}
	default:
		return 'e', nil
	}
}
