package main

import (
	"bytes"
	"image"
	"image/png"
	"os"
	_ "image/jpeg"
	_ "image/png"

	"github.com/leaanthony/winicon"
)

func main() {
	in, err := os.Open("c:/ResultProxyPC/public/logo.png")
	if err != nil {
		panic(err)
	}
	img, _, err := image.Decode(in)
	_ = in.Close()
	if err != nil {
		panic(err)
	}

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		panic(err)
	}
	if err := os.WriteFile("c:/ResultProxyPC/public/logo.png", pngBuf.Bytes(), 0o644); err != nil {
		panic(err)
	}
	if err := os.MkdirAll("c:/ResultProxyPC/build", 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile("c:/ResultProxyPC/build/appicon.png", pngBuf.Bytes(), 0o644); err != nil {
		panic(err)
	}

	if err := os.MkdirAll("c:/ResultProxyPC/build/windows", 0o755); err != nil {
		panic(err)
	}
	out, err := os.Create("c:/ResultProxyPC/build/windows/icon.ico")
	if err != nil {
		panic(err)
	}
	defer out.Close()

	if err := winicon.GenerateIcon(bytes.NewReader(pngBuf.Bytes()), out, []int{256, 128, 64, 48, 32, 16}); err != nil {
		panic(err)
	}
}
