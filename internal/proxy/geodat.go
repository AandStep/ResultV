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

// Reader for the v2ray/xray geo databases — geosite.dat and geoip.dat.
//
// Why this file exists: routing profiles arrive from panels and deep links as
// xray rule sets, and their contents are almost entirely `geosite:<category>` /
// `geoip:<category>` references into these two files. sing-box dropped .dat
// support in 1.8, so the categories have to be resolved here and compiled into
// the source-format rule_set cache the router already consumes
// (see routinglist.go). Left unresolved, such a profile imports empty.
//
// The wire format is protobuf, but no protobuf dependency or generated code is
// pulled in for it: the schemas below are four tiny messages and never change
// (they are frozen by the file format, not by a library version). Hand-reading
// them keeps the .proto, the codegen step and a v2ray-core dependency out of
// the build.
//
//	GeoSiteList { repeated GeoSite entry = 1 }
//	GeoSite     { string country_code = 1; repeated Domain domain = 2 }
//	Domain      { Type type = 1; string value = 2; repeated Attribute attr = 3 }
//	Domain.Type { Plain = 0; Regex = 1; Domain = 2; Full = 3 }
//
//	GeoIPList { repeated GeoIP entry = 1 }
//	GeoIP     { string country_code = 1; repeated CIDR cidr = 2; bool inverse = 3 }
//	CIDR      { bytes ip = 1; uint32 prefix = 2 }

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// Domain.Type values, as stored in the file.
const (
	geoDomainPlain  = 0 // substring ("keyword") match
	geoDomainRegex  = 1
	geoDomainDomain = 2 // the host and everything under it
	geoDomainFull   = 3 // this host exactly
)

// GeoDomain is one entry of a geosite category, reduced to what a sing-box
// rule-set can express. Plain and Regex entries never reach here — see
// ParseGeoSiteDat.
type GeoDomain struct {
	Value string
	// Exact distinguishes Full (the host itself) from Domain (the host and its
	// sub-domains). Collapsing the two would silently widen a rule: `full:`
	// entries exist precisely to NOT catch sub-domains.
	Exact bool
}

// ErrGeoDatMalformed reports a file that is not a readable geo database.
var ErrGeoDatMalformed = errors.New("geo database is malformed")

// ParseGeoSiteDat reads geosite.dat into category → domains. Category keys are
// lower-cased: the file stores them upper-case ("WHITELIST"), while rules
// reference them lower-case ("geosite:whitelist").
//
// Plain (keyword) and Regex entries are dropped: a sing-box rule-set has no
// equivalent, and admitting them as suffixes would match hosts the author never
// listed. The count of dropped entries is returned so a caller can say so out
// loud rather than silently importing a shorter list.
func ParseGeoSiteDat(raw []byte) (map[string][]GeoDomain, int, error) {
	out := make(map[string][]GeoDomain)
	dropped := 0
	err := eachField(raw, func(field int, wire int, value []byte) error {
		if field != 1 || wire != wireBytes {
			return nil // GeoSiteList has no other fields; ignore future ones.
		}
		name, domains, skipped, err := parseGeoSite(value)
		if err != nil {
			return err
		}
		dropped += skipped
		if name == "" {
			return nil
		}
		// A category may legitimately appear more than once; append, don't
		// replace, so the second block is not lost.
		out[name] = append(out[name], domains...)
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	if len(out) == 0 {
		return nil, 0, fmt.Errorf("%w: no geosite categories found", ErrGeoDatMalformed)
	}
	return out, dropped, nil
}

func parseGeoSite(buf []byte) (name string, domains []GeoDomain, dropped int, err error) {
	err = eachField(buf, func(field int, wire int, value []byte) error {
		switch {
		case field == 1 && wire == wireBytes:
			name = strings.ToLower(strings.TrimSpace(string(value)))
		case field == 2 && wire == wireBytes:
			d, ok, derr := parseGeoDomain(value)
			if derr != nil {
				return derr
			}
			if !ok {
				dropped++
				return nil
			}
			domains = append(domains, d)
		}
		return nil
	})
	return name, domains, dropped, err
}

// parseGeoDomain returns ok=false for entries a rule-set cannot express.
func parseGeoDomain(buf []byte) (GeoDomain, bool, error) {
	kind := uint64(geoDomainDomain) // proto3: an absent type field means 0…
	sawType := false
	var value string
	err := eachField(buf, func(field int, wire int, raw []byte) error {
		switch {
		case field == 1 && wire == wireVarint:
			kind = decodeVarintBytes(raw)
			sawType = true
		case field == 2 && wire == wireBytes:
			value = strings.ToLower(strings.TrimSpace(string(raw)))
		}
		return nil
	})
	if err != nil {
		return GeoDomain{}, false, err
	}
	// …but 0 is Plain, not Domain. Real files always write the field for
	// non-zero types and omit it for Plain, so an absent field means Plain.
	if !sawType {
		kind = geoDomainPlain
	}
	if value == "" {
		return GeoDomain{}, false, nil
	}
	switch kind {
	case geoDomainDomain:
		return GeoDomain{Value: value}, true, nil
	case geoDomainFull:
		return GeoDomain{Value: value, Exact: true}, true, nil
	case geoDomainPlain, geoDomainRegex:
		return GeoDomain{}, false, nil
	default:
		return GeoDomain{}, false, nil
	}
}

// ParseGeoIPDat reads geoip.dat into category → CIDRs, keys lower-cased for the
// same reason as ParseGeoSiteDat.
//
// A category carrying inverse_match is dropped: it means "everything except
// these", which a rule-set cannot express, and importing the listed prefixes
// as-is would invert the author's intent. Such categories are named in the
// returned slice so the caller can report them.
func ParseGeoIPDat(raw []byte) (map[string][]string, []string, error) {
	out := make(map[string][]string)
	var inverted []string
	err := eachField(raw, func(field int, wire int, value []byte) error {
		if field != 1 || wire != wireBytes {
			return nil
		}
		name, cidrs, inverse, err := parseGeoIP(value)
		if err != nil {
			return err
		}
		if name == "" {
			return nil
		}
		if inverse {
			inverted = append(inverted, name)
			return nil
		}
		out[name] = append(out[name], cidrs...)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(out) == 0 && len(inverted) == 0 {
		return nil, nil, fmt.Errorf("%w: no geoip categories found", ErrGeoDatMalformed)
	}
	return out, inverted, nil
}

func parseGeoIP(buf []byte) (name string, cidrs []string, inverse bool, err error) {
	err = eachField(buf, func(field int, wire int, value []byte) error {
		switch {
		case field == 1 && wire == wireBytes:
			name = strings.ToLower(strings.TrimSpace(string(value)))
		case field == 2 && wire == wireBytes:
			c, ok, cerr := parseGeoCIDR(value)
			if cerr != nil {
				return cerr
			}
			if ok {
				cidrs = append(cidrs, c)
			}
		case field == 3 && wire == wireVarint:
			inverse = decodeVarintBytes(value) != 0
		}
		return nil
	})
	return name, cidrs, inverse, err
}

func parseGeoCIDR(buf []byte) (string, bool, error) {
	var ip []byte
	var prefix uint64
	sawPrefix := false
	err := eachField(buf, func(field int, wire int, value []byte) error {
		switch {
		case field == 1 && wire == wireBytes:
			ip = value
		case field == 2 && wire == wireVarint:
			prefix = decodeVarintBytes(value)
			sawPrefix = true
		}
		return nil
	})
	if err != nil {
		return "", false, err
	}
	// The file stores the address as raw bytes: 4 for v4, 16 for v6.
	if len(ip) != net.IPv4len && len(ip) != net.IPv6len {
		return "", false, nil
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return "", false, nil
	}
	// A 16-byte v4-mapped address must be written back as v4, or the prefix
	// length (counted against 32 bits by the author) lands on the wrong axis.
	addr = addr.Unmap()
	if !sawPrefix {
		prefix = uint64(addr.BitLen()) // absent prefix = single host
	}
	if int(prefix) > addr.BitLen() {
		return "", false, nil
	}
	return netip.PrefixFrom(addr, int(prefix)).String(), true, nil
}

// --- minimal protobuf wire reader ---------------------------------------

const (
	wireVarint = 0
	wireBytes  = 2
	wire64     = 1
	wire32     = 5
)

// eachField walks top-level fields of one protobuf message. For varint fields
// the raw varint bytes are handed over (decode with decodeVarintBytes); for
// length-delimited fields, the payload.
func eachField(buf []byte, fn func(field int, wire int, value []byte) error) error {
	for len(buf) > 0 {
		tag, n := decodeVarint(buf)
		if n <= 0 {
			return fmt.Errorf("%w: bad field tag", ErrGeoDatMalformed)
		}
		buf = buf[n:]
		field := int(tag >> 3)
		wire := int(tag & 0x7)
		if field == 0 {
			return fmt.Errorf("%w: field number 0", ErrGeoDatMalformed)
		}
		switch wire {
		case wireVarint:
			_, vn := decodeVarint(buf)
			if vn <= 0 {
				return fmt.Errorf("%w: bad varint", ErrGeoDatMalformed)
			}
			if err := fn(field, wire, buf[:vn]); err != nil {
				return err
			}
			buf = buf[vn:]
		case wireBytes:
			length, ln := decodeVarint(buf)
			if ln <= 0 {
				return fmt.Errorf("%w: bad length prefix", ErrGeoDatMalformed)
			}
			buf = buf[ln:]
			if length > uint64(len(buf)) {
				return fmt.Errorf("%w: length %d exceeds %d remaining", ErrGeoDatMalformed, length, len(buf))
			}
			if err := fn(field, wire, buf[:length]); err != nil {
				return err
			}
			buf = buf[length:]
		case wire64:
			if len(buf) < 8 {
				return fmt.Errorf("%w: truncated 64-bit field", ErrGeoDatMalformed)
			}
			buf = buf[8:]
		case wire32:
			if len(buf) < 4 {
				return fmt.Errorf("%w: truncated 32-bit field", ErrGeoDatMalformed)
			}
			buf = buf[4:]
		default:
			// Group wire types (3, 4) were removed from proto3 and never appear
			// in these files; without a length there is no way to skip one.
			return fmt.Errorf("%w: unsupported wire type %d", ErrGeoDatMalformed, wire)
		}
	}
	return nil
}

func decodeVarint(buf []byte) (uint64, int) {
	var value uint64
	var shift uint
	for i := 0; i < len(buf); i++ {
		if i == 10 { // a varint is at most 10 bytes
			return 0, -1
		}
		b := buf[i]
		value |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return value, i + 1
		}
		shift += 7
	}
	return 0, -1
}

func decodeVarintBytes(raw []byte) uint64 {
	v, n := decodeVarint(raw)
	if n <= 0 {
		return 0
	}
	return v
}
