// Package proxy implements a MITM proxy that uses urlfilter to filter content.
// TODO(ameshkov): extract to a submodule
package proxy

import (
	"fmt"
	"time"

	"github.com/AdguardTeam/golibs/log"
	"github.com/AdguardTeam/gomitmproxy"
	"github.com/AdguardTeam/urlfilter"
	"github.com/AdguardTeam/urlfilter/filterlist"
	"github.com/AdguardTeam/urlfilter/rules"
)

const (
	sessionPropKey    = "session"
	requestBlockedKey = "blocked"
)

var defaultInjectionsHost = "injections.adguard.org"

// Config contains the MITM proxy configuration
type Config struct {
	// Config of the MITM proxy
	ProxyConfig gomitmproxy.Config

	// Paths to the filtering rules
	FiltersPaths map[rules.ListID]string

	// InjectionHost is used for injecting custom CSS/JS into web pages.
	//
	// Here's how it works:
	// * The proxy injects `<script src="//INJECTIONS_HOST/content-script.js?domain=HOSTNAME&flags=FLAGS"></script>`
	// * Depending on the FLAGS and the HOSTNAME, it either injects cosmetic rules or not
	// * Proxy handles requests to this host
	// * The content script content depends on the FLAGS value
	InjectionHost string

	// Engine, when non-nil, is used verbatim instead of building one from
	// FiltersPaths. ResultV addition: parsing the filter lists is the expensive
	// part of NewServer, so the Manager builds the engine once and reuses it
	// across proxy restarts (see internal/filter Manager.cachedEngine) to keep
	// the browser ad-block attach fast on every connect after the first.
	Engine *urlfilter.Engine

	// ScriptletIndex, when non-nil, is used verbatim instead of building one
	// from FiltersPaths — mirrors Engine above. It indexes the `#%#`/`#@%#`
	// cosmetic-JS rules that urlfilter's own parser rejects (see
	// scriptletindex.go), so buildContentScript can still serve them to the
	// browser. Same caching rationale as Engine: the Manager builds it once
	// and reuses it across proxy restarts.
	ScriptletIndex *ScriptletIndex

	// If true, we will serve the content-script compressed
	// This is useful for the case when the proxy is on a public server,
	// as it saves some data.
	CompressContentScript bool
}

// String - server's configuration description
func (c *Config) String() string {
	str := ""
	str += fmt.Sprintf("Listen addr: %s\n", c.ProxyConfig.ListenAddr.String())
	str += fmt.Sprintf("MITM status: %v\n", c.ProxyConfig.MITMConfig != nil)
	str += fmt.Sprintf("Run as HTTPS proxy: %v\n", c.ProxyConfig.TLSConfig != nil)

	if c.ProxyConfig.Username != "" {
		str += fmt.Sprintf("Proxy auth: %s/%s\n", c.ProxyConfig.Username, c.ProxyConfig.Password)
	}
	if c.ProxyConfig.APIHost != "" {
		str += fmt.Sprintf("API host: %s\n", c.ProxyConfig.APIHost)
	}

	if len(c.FiltersPaths) > 0 {
		str += fmt.Sprintf("Filter lists: %d\n", len(c.FiltersPaths))
		for id, v := range c.FiltersPaths {
			str += fmt.Sprintf("%d: %q\n", id, v)
		}
	}

	return str
}

// Server contains the current server state
type Server struct {
	// the MITM proxy server instance
	proxyServer *gomitmproxy.Proxy

	// filtering engine
	engine *urlfilter.Engine

	// scriptletIndex indexes `#%#`/`#@%#` cosmetic-JS rules; nil disables
	// scriptlet injection (degraded mode, see NewServer).
	scriptletIndex *ScriptletIndex

	// time when the server was created
	createdAt time.Time

	Config // Server configuration
}

// NewServer creates a new instance of the MITM server
func NewServer(config Config) (*Server, error) {
	log.Info("Initializing the proxy server:\n%s", config.String())

	if config.InjectionHost == "" {
		config.InjectionHost = defaultInjectionsHost
	}

	s := &Server{
		createdAt: time.Now(),
		Config:    config,
	}

	// ResultV: reuse a pre-built engine when the caller supplies one (the
	// Manager caches it across proxy restarts), else build from FiltersPaths.
	engine := config.Engine
	if engine == nil {
		var err error
		engine, err = buildEngine(config)
		if err != nil {
			return nil, err
		}
	}

	s.engine = engine

	// ResultV: reuse a pre-built scriptlet index when supplied, else build
	// from FiltersPaths. An index build error is logged and degrades to no
	// scriptlets — it must never fail NewServer or take down the page, since
	// scriptlet execution is a best-effort ad-block enhancement, not a
	// requirement for the proxy to serve traffic.
	scriptletIndex := config.ScriptletIndex
	if scriptletIndex == nil {
		var err error
		scriptletIndex, err = BuildScriptletIndex(config.FiltersPaths)
		if err != nil {
			log.Error("failed to build scriptlet index, scriptlets disabled: %v", err)
			scriptletIndex = nil
		}
	}
	s.scriptletIndex = scriptletIndex

	s.ProxyConfig.OnRequest = s.onRequest
	s.ProxyConfig.OnResponse = s.onResponse
	s.ProxyConfig.OnConnect = s.onConnect
	s.proxyServer = gomitmproxy.NewProxy(s.ProxyConfig)
	return s, nil
}

// Start starts the proxy server
func (s *Server) Start() error {
	return s.proxyServer.Start()
}

// Close stops the proxy server
func (s *Server) Close() {
	s.proxyServer.Close()
}

// BuildEngine builds a urlfilter engine from the given filter files. Exported
// (ResultV addition) so the Manager can build the engine once and cache it
// across proxy restarts — parsing the lists is the expensive part of NewServer.
func BuildEngine(paths map[rules.ListID]string) (*urlfilter.Engine, error) {
	return buildEngine(Config{FiltersPaths: paths})
}

// buildEngine builds a new network engine
func buildEngine(config Config) (*urlfilter.Engine, error) {
	var lists []filterlist.Interface

	for filterID, path := range config.FiltersPaths {
		list, err := filterlist.NewFile(&filterlist.FileConfig{
			Path: path,
			ID:   filterID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create rule list %d: %w", filterID, err)
		}
		lists = append(lists, list)
	}

	ruleStorage, err := filterlist.NewRuleStorage(lists)
	if err != nil {
		return nil, fmt.Errorf("cannot initialize rule storage: %w", err)
	}

	return urlfilter.NewEngine(ruleStorage), nil
}
