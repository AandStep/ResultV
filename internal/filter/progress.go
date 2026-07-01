// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

// UpdateProgress is emitted to the UI during filter list updates.
type UpdateProgress struct {
	Phase   string `json:"phase"` // lists | done | error
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Item    string `json:"item,omitempty"`
	Message string `json:"message,omitempty"`
}
