// Copyright (C) 2026 ResultProxy
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed wails.json
var embeddedWailsJSON []byte

var (
	productVersionCached string
	productVersionOnce   sync.Once
)

func productVersionFromWailsJSON() string {
	productVersionOnce.Do(func() {
		var cfg struct {
			Info struct {
				ProductVersion string `json:"productVersion"`
			} `json:"info"`
		}
		if err := json.Unmarshal(embeddedWailsJSON, &cfg); err != nil || cfg.Info.ProductVersion == "" {
			productVersionCached = "0.0.0"
			return
		}
		productVersionCached = cfg.Info.ProductVersion
	})
	return productVersionCached
}
