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

//go:build windows

package system

import (
	"fmt"
	"strconv"
	"strings"
)

func GetNetworkTraffic() TrafficStats {
	out, err := command("netstat", "-e").Output()
	if err != nil {
		return TrafficStats{}
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) >= 3 {
			received, err1 := strconv.ParseInt(parts[len(parts)-2], 10, 64)
			sent, err2 := strconv.ParseInt(parts[len(parts)-1], 10, 64)
			if err1 == nil && err2 == nil && received > 0 {
				return TrafficStats{Received: received, Sent: sent}
			}
		}
	}

	return TrafficStats{}
}

func IsAdmin() bool {
	err := command("net", "session").Run()
	return err == nil
}

func RestartAsAdmin(exePath string, args ...string) error {
	psArgs := fmt.Sprintf(`Start-Process -FilePath '%s' -Verb RunAs`, exePath)
	if len(args) > 0 {
		psArgs += fmt.Sprintf(` -ArgumentList '%s'`, strings.Join(args, " "))
	}
	return command("powershell", "-Command", psArgs).Start()
}
