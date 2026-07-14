// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build windows

package system

import (
	"runtime"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func subscriptionDeviceInfo() DeviceInfo {
	info := fallbackDeviceInfo()
	info.Platform = "Windows"
	if version := windowsOSVersion(); version != "" {
		info.OSVersion = version
	}
	if model := windowsDeviceModel(); model != "" {
		info.Model = model
	}
	return info
}

func windowsOSVersion() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer k.Close()

	product, _, _ := k.GetStringValue("ProductName")
	display, _, _ := k.GetStringValue("DisplayVersion")
	build, _, _ := k.GetStringValue("CurrentBuildNumber")
	if build == "" {
		build, _, _ = k.GetStringValue("CurrentBuild")
	}

	major := "Windows"
	if strings.Contains(product, "11") || buildAtLeast(build, 22000) {
		major = "11"
	} else if strings.Contains(product, "10") {
		major = "10"
	}
	if display != "" && build != "" {
		return major + "_" + display + "." + build
	}
	if build != "" {
		return major + "_" + build
	}
	return strings.ReplaceAll(strings.TrimSpace(product), " ", "_")
}

func windowsDeviceModel() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "Windows_" + runtime.GOARCH
	}
	defer k.Close()

	for _, name := range []string{"SystemProductName", "BaseBoardProduct"} {
		v, _, err := k.GetStringValue(name)
		v = strings.TrimSpace(v)
		if err == nil && v != "" && !strings.EqualFold(v, "System Product Name") {
			return strings.ReplaceAll(v, " ", "_")
		}
	}
	return "Windows_" + runtime.GOARCH
}

func buildAtLeast(build string, min int) bool {
	n := 0
	for _, r := range build {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n >= min
}
