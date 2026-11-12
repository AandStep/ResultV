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

package proxy

/*
#cgo LDFLAGS: -framework CoreFoundation -framework SystemConfiguration
#include <SystemConfiguration/SystemConfiguration.h>
#include <stdlib.h>

static SCDynamicStoreRef resultvInstallDNS(const char *address, int *status) {
	SCDynamicStoreRef store = SCDynamicStoreCreate(NULL, CFSTR("ResultV DNS"), NULL, NULL);
	if (store == NULL) {
		*status = SCError();
		return NULL;
	}

	CFStringRef server = CFStringCreateWithCString(NULL, address, kCFStringEncodingUTF8);
	const void *serverValues[] = {server};
	const void *domains[] = {CFSTR("")};
	CFArrayRef servers = CFArrayCreate(NULL, serverValues, 1, &kCFTypeArrayCallBacks);
	CFArrayRef matchDomains = CFArrayCreate(NULL, domains, 1, &kCFTypeArrayCallBacks);
	int priority = 1;
	CFNumberRef order = CFNumberCreate(NULL, kCFNumberIntType, &priority);
	const void *keys[] = {
		kSCPropNetDNSServerAddresses,
		kSCPropNetDNSSupplementalMatchDomains,
		kSCPropNetDNSSearchOrder,
	};
	const void *values[] = {servers, matchDomains, order};
	CFDictionaryRef config = CFDictionaryCreate(NULL, keys, values, 3,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);

	Boolean added = SCDynamicStoreAddTemporaryValue(store,
		CFSTR("State:/Network/Service/3BB68896-7B75-497A-93BB-49F1DC75B6AD/DNS"), config);
	if (!added) {
		*status = SCError();
		CFRelease(store);
		store = NULL;
	}
	CFRelease(config);
	CFRelease(order);
	CFRelease(matchDomains);
	CFRelease(servers);
	CFRelease(server);
	return store;
}

static void resultvReleaseDNS(SCDynamicStoreRef store) {
	CFRelease(store);
}
*/
import "C"

import (
	"fmt"
	"net/netip"
	"sync"
	"unsafe"
)

// darwinSystemDNS owns a temporary default resolver in configd. Closing the
// store session removes it, including when the owning process is killed.
type darwinSystemDNS struct {
	mu    sync.Mutex
	store C.SCDynamicStoreRef
}

func newSystemDNS() SystemDNS { return &darwinSystemDNS{} }

func (*darwinSystemDNS) Override([]string) error            { return nil }
func (*darwinSystemDNS) SnapshotExists() bool               { return false }
func (*darwinSystemDNS) RestoreCommands() ([]string, error) { return nil, nil }
func (*darwinSystemDNS) DeleteSnapshot() error              { return nil }

func (d *darwinSystemDNS) OverrideTunnelAdapter(_ string, dnsIP string) error {
	addr, err := netip.ParseAddr(dnsIP)
	if err != nil || !addr.Is4() {
		return fmt.Errorf("invalid IPv4 tunnel resolver: %q", dnsIP)
	}
	address := C.CString(addr.String())
	defer C.free(unsafe.Pointer(address))

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.store != 0 {
		return fmt.Errorf("the ResultV DNS session is already active")
	}
	var status C.int
	store := C.resultvInstallDNS(address, &status)
	if store == 0 {
		return fmt.Errorf("register macOS tunnel resolver: %s", C.GoString(C.SCErrorString(status)))
	}
	d.store = store
	return nil
}

func (d *darwinSystemDNS) Restore() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.store != 0 {
		C.resultvReleaseDNS(d.store)
		d.store = 0
	}
	return nil
}
