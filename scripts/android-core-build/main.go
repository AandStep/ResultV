// Command android-core-build runs an AAR build or test against the pinned
// sing-box dependency with the Android DNS fix applied to a temporary copy.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const singBox = "github.com/sagernet/sing-box"

// The router calls ExchangeAsync from the TUN packet reader. Fallback's
// upstream lookup can take several seconds and must run independently. Once
// the pinned core does that itself, remove this patch and run the regression.
const blockingExchange = `func (t *Transport) ExchangeAsync(ctx context.Context, message *mDNS.Msg, callback func(response *mDNS.Msg, err error)) {
	callback(t.Exchange(ctx, message))
}`

const asyncExchange = `func (t *Transport) ExchangeAsync(ctx context.Context, message *mDNS.Msg, callback func(response *mDNS.Msg, err error)) {
	go func() {
		callback(t.Exchange(ctx, message))
	}()
}`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: android-core-build <command> [args...]")
		os.Exit(2)
	}
	if err := runWithPatchedCore(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "build with Android core:", err)
		os.Exit(1)
	}
}

func runWithPatchedCore(args []string) error {
	if err := os.Setenv("GOWORK", "off"); err != nil {
		return err
	}
	if _, err := goOutput("mod", "download", singBox); err != nil {
		return err
	}
	metadata, err := goOutput("list", "-m", "-json", singBox)
	if err != nil {
		return err
	}
	var dependency struct{ Dir, Version string }
	if err := json.Unmarshal(metadata, &dependency); err != nil {
		return err
	}
	if dependency.Dir == "" || dependency.Version == "" {
		return fmt.Errorf("could not resolve pinned dependency %s", singBox)
	}

	// Go refuses an overlay inside GOMODCACHE. Copy the selected module outside
	// it and select the copy via a temporary workspace. gomobile also calls Go
	// from scratch directories, where an alternate modfile would be rejected.
	tempRoot, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return err
	}
	scratch, err := os.MkdirTemp(tempRoot, "resultv-android-core-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	coreDir := filepath.Join(scratch, "sing-box")
	if err := os.CopyFS(coreDir, os.DirFS(dependency.Dir)); err != nil {
		return err
	}
	if err := patchFallback(coreDir); err != nil {
		return err
	}
	// The SOCKS5 UDP selector must start on the VPN until the new session's
	// probe confirms UDP. Keep this selector's choice out of the DNS cache DB.
	if err := patchCoreFile(
		filepath.Join(coreDir, "protocol", "group", "selector.go"),
		`if s.Tag() != "" {`, `if s.Tag() != "" && s.Tag() != "socks5-exit-udp" {`, 2,
	); err != nil {
		return err
	}
	// A sniffed hostname must be resolved by the actual TCP exit, even if
	// the user's device already resolved it for the TUN's first hop.
	if err := patchCoreFile(filepath.Join(coreDir, "protocol", "socks", "outbound.go"),
		`	metadata.Destination = destination
	switch N.NetworkName(network) {`,
		`	if h.Tag() == "socks5-exit" && N.NetworkName(network) == N.NetworkTCP && destination.IsIP() && M.IsDomainName(metadata.Domain) {
		destination = M.Socksaddr{Fqdn: metadata.Domain, Port: destination.Port}
	}
	metadata.Destination = destination
	switch N.NetworkName(network) {`, 1); err != nil {
		return err
	}
	repoRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := goWorkspaceOutput(scratch, "work", "init", repoRoot); err != nil {
		return err
	}
	workFile := filepath.Join(scratch, "go.work")
	if err := goWorkspaceOutput(scratch, "work", "edit", "-replace="+singBox+"@"+dependency.Version+"="+coreDir, workFile); err != nil {
		return err
	}
	command := exec.Command(args[0], args[1:]...)
	command.Env = append(os.Environ(), "GOWORK="+workFile)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}

func patchFallback(coreDir string) error {
	return patchCoreFile(
		filepath.Join(coreDir, "dns", "transport", "fallback", "fallback.go"),
		blockingExchange, asyncExchange, 1,
	)
}

func patchCoreFile(path, before, after string, want int) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	contents = bytes.ReplaceAll(contents, []byte("\r\n"), []byte("\n"))
	if bytes.Count(contents, []byte(before)) != want {
		return fmt.Errorf("%s changed: review the patch against the selected sing-box version", path)
	}
	return os.WriteFile(path, bytes.ReplaceAll(contents, []byte(before), []byte(after)), 0o600)
}

func goWorkspaceOutput(dir string, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go %s: %w: %s", strings.Join(args, " "), err, bytes.TrimSpace(out))
	}
	return nil
}

func goOutput(args ...string) ([]byte, error) {
	output, err := exec.Command("go", args...).Output()
	if err == nil {
		return output, nil
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return nil, fmt.Errorf("go %s: %w: %s", strings.Join(args, " "), err, bytes.TrimSpace(exit.Stderr))
	}
	return nil, fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
}
