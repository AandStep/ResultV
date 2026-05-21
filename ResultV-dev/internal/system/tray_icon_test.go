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
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestPngToICO(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	img.Set(1, 0, color.NRGBA{R: 0, G: 255, B: 0, A: 255})
	img.Set(0, 1, color.NRGBA{R: 0, G: 0, B: 255, A: 255})
	img.Set(1, 1, color.NRGBA{R: 255, G: 255, B: 0, A: 255})
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	ico, err := pngToICO(pngBuf.Bytes(), 16)
	if err != nil {
		t.Fatalf("pngToICO failed: %v", err)
	}
	if len(ico) <= pngBuf.Len() {
		t.Fatalf("expected ico payload with header, got len=%d", len(ico))
	}
	if ico[0] != 0x00 || ico[1] != 0x00 || ico[2] != 0x01 || ico[3] != 0x00 {
		t.Fatalf("invalid ico header: %v", ico[:4])
	}
}
