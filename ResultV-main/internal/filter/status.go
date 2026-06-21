// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

// Status is exposed to the UI via Wails.
type Status struct {
	Enabled            bool   `json:"enabled"`
	FilterCount        int    `json:"filterCount"`
	RuleSetsReady      int    `json:"ruleSetsReady"`
	RuleSetsTotal      int    `json:"ruleSetsTotal"`
	LastUpdatedUnix    int64  `json:"lastUpdatedUnix"`
	LastError          string `json:"lastError,omitempty"`
	CAInstalled        bool   `json:"caInstalled"`
	NetworkBlocked     uint64 `json:"networkBlocked"`
	CosmeticBlocked    uint64 `json:"cosmeticBlocked"`
	UpdateInProgress   bool   `json:"updateInProgress"`
	UpdatePhase        string `json:"updatePhase,omitempty"`
	UpdateCurrent      int    `json:"updateCurrent"`
	UpdateTotal        int    `json:"updateTotal"`
	UpdateItem         string `json:"updateItem,omitempty"`
	NetworkBlockActive bool   `json:"networkBlockActive"`
	NeedsReconnect     bool   `json:"needsReconnect"`
}
