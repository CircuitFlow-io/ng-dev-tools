package doctor

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	slowResponse       = time.Second
	dnsProbeHost       = "github.com"
	defaultNpmRegistry = "https://registry.npmjs.org/"
)

// endpoint is a service the daily workflow downloads from.
type endpoint struct {
	name string
	url  string
}

var endpoints = []endpoint{
	{"GitHub", "https://github.com"},
	{"npm registry", "https://registry.npmjs.org"},
	{"Go module proxy", "https://proxy.golang.org"},
	{"CocoaPods CDN", "https://cdn.cocoapods.org"},
	{"Google Maven (Android)", "https://dl.google.com"},
	{"Anthropic API", "https://api.anthropic.com"},
}

var proxyVariables = []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"}

func networkChecks() []Check {
	checks := []Check{
		{Name: "DNS", Group: GroupNetwork, NeedsNetwork: true, Run: checkDNS},
	}
	for _, e := range endpoints {
		checks = append(checks, Check{Name: e.name, Group: GroupNetwork, NeedsNetwork: true, Run: e.check})
	}
	return append(checks,
		Check{Name: "Proxy variables", Group: GroupNetwork, Run: checkProxyVariables},
		Check{Name: "npm registry setting", Group: GroupNetwork, Run: checkNpmRegistry},
	)
}

func checkDNS(ctx context.Context, env Env) Result {
	if err := env.Probe.LookupHost(ctx, dnsProbeHost); err != nil {
		return fail("could not resolve "+dnsProbeHost, "check your connection, VPN or DNS settings").with(err.Error())
	}
	return pass("resolves " + dnsProbeHost)
}

func (e endpoint) check(ctx context.Context, env Env) Result {
	latency, err := env.Probe.Reach(ctx, e.url)
	if err != nil {
		return fail("unreachable", "check your connection, VPN or proxy").with(err.Error())
	}
	summary := fmt.Sprintf("%s in %s", strings.TrimPrefix(e.url, "https://"), latency.Round(time.Millisecond))
	if latency > slowResponse {
		return warn("slow: "+summary, "")
	}
	return pass(summary)
}

// checkProxyVariables names the variables but never prints their values, which often hold credentials.
func checkProxyVariables(_ context.Context, env Env) Result {
	var set []string
	for _, key := range proxyVariables {
		if env.Getenv(key) != "" {
			set = append(set, key)
		}
	}
	if len(set) > 0 {
		return warn(strings.Join(set, ", ")+" set; every download goes through a proxy", "unset them if you are not behind a corporate proxy")
	}
	return pass("none set")
}

func checkNpmRegistry(ctx context.Context, env Env) Result {
	if !env.installed("npm") {
		return skip("npm is not installed")
	}
	registry, err := env.output(ctx, "npm", "config", "get", "registry")
	if err != nil {
		return warn("could not read the npm config", "").with(errorLine(err))
	}
	if registry != defaultNpmRegistry {
		return warn("npm installs from "+registry, "npm config delete registry")
	}
	return pass(registry)
}
