// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"bytes"
	"fmt"

	"github.com/sagernet/sing-box/common/srs"
)

// validateSRS parses data with sing-box's own rule-set reader — the exact code
// path the engine uses at load time. A nil return therefore guarantees sing-box
// will accept the file. It catches the field failure mode a bare size check
// cannot: a truncated download whose zlib stream fails its checksum on read
// ("restore cached rule-set: read rule[0] zlib invalid checksum"), which
// otherwise bricks rule-set startup.
//
// It lives in its own file rather than beside the ad lists because Smart mode
// validates its rule-sets with it too (smart_ruleset.go), and the no_adblock
// build keeps Smart while dropping the lists.
func validateSRS(data []byte) error {
	if _, err := srs.Read(bytes.NewReader(data), false); err != nil {
		return fmt.Errorf("invalid SRS: %w", err)
	}
	return nil
}
