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

package system

import (
	"reflect"

	"github.com/getlantern/systray"
)

type serverClickBinding struct {
	proxyID string
	ch      <-chan struct{}
}

type trayClickDispatcher struct {
	updates chan []serverClickBinding
	stopCh  chan struct{}
	doneCh  chan struct{}
}

func newTrayClickDispatcher() *trayClickDispatcher {
	return &trayClickDispatcher{
		updates: make(chan []serverClickBinding, 1),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

func (d *trayClickDispatcher) start(onClick func(string)) {
	go func() {
		defer close(d.doneCh)
		bindings := make([]serverClickBinding, 0)
		for {
			if len(bindings) == 0 {
				select {
				case <-d.stopCh:
					return
				case next := <-d.updates:
					bindings = next
				}
				continue
			}

			cases := make([]reflect.SelectCase, 0, len(bindings)+2)
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(d.stopCh)})
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(d.updates)})
			for _, binding := range bindings {
				cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(binding.ch)})
			}

			chosen, recv, ok := reflect.Select(cases)
			if chosen == 0 {
				return
			}
			if chosen == 1 {
				if next, castOK := recv.Interface().([]serverClickBinding); castOK {
					bindings = next
				}
				continue
			}
			if !ok {
				continue
			}
			onClick(bindings[chosen-2].proxyID)
		}
	}()
}

func (d *trayClickDispatcher) update(bindings []serverClickBinding) {
	select {
	case d.updates <- bindings:
		return
	default:
	}
	select {
	case <-d.updates:
	default:
	}
	d.updates <- bindings
}

func (d *trayClickDispatcher) stop() {
	select {
	case <-d.stopCh:
	default:
		close(d.stopCh)
	}
	<-d.doneCh
}

func buildServerClickBindings(serverItems map[string]*systray.MenuItem) []serverClickBinding {
	bindings := make([]serverClickBinding, 0, len(serverItems))
	for proxyID, item := range serverItems {
		if item == nil {
			continue
		}
		bindings = append(bindings, serverClickBinding{proxyID: proxyID, ch: item.ClickedCh})
	}
	return bindings
}
