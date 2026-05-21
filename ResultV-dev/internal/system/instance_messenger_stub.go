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

//go:build !windows


package system


func InitSingletonMessenger(onActivate func(payload string)) (cleanup func()) {
	return func() {}
}

// ExtractDeepLinkArg scans process args for the first resultv: URL.
func ExtractDeepLinkArg(args []string) string {
	const scheme = "resultv:"
	if len(args) <= 1 {
		return ""
	}
	for _, a := range args[1:] {
		s := a
		if len(s) > len(scheme) {
			lower := s[:len(scheme)]
			match := true
			for i := 0; i < len(scheme); i++ {
				c := lower[i]
				if c >= 'A' && c <= 'Z' {
					c += 'a' - 'A'
				}
				if c != scheme[i] {
					match = false
					break
				}
			}
			if match {
				return s
			}
		}
	}
	return ""
}
