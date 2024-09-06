package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

const (
	MATCH_ALL_DOMAINS string = "*"
)

type BackendConfigList []*BackendConfig

type BackendConfig struct {
	Domain         string `json:"domain"   yaml:"domain"`
	Basepath       string `json:"basepath" yaml:"basepath"`
	BackendBaseUrl string `json:"backend"  yaml:"backend"`
}

func NewBackendConfig(domain, basepath, backendBaseUrl string) *BackendConfig {
	return &BackendConfig{
		Domain:         normalizeDomain(domain),
		Basepath:       basepath,
		BackendBaseUrl: backendBaseUrl,
	}
}

// TODO not sure, maybe we should use "" as "all domains"
// and only print "*" when we communicate with the user
func normalizeDomain(d string) string {
	if d == "" {
		return MATCH_ALL_DOMAINS
	}
	return d
}

func (bc *BackendConfig) NormDomain() string {
	return normalizeDomain(bc.Domain)
}

func (bc *BackendConfig) MatchesAllDomains() bool {
	return bc.NormDomain() == MATCH_ALL_DOMAINS
}

// TODO: maybe use DeepEquals
func (a *BackendConfig) Equals(b *BackendConfig) bool {
	if b == nil {
		return false
	}
	if a == b {
		return true
	}
	aDomain := normalizeDomain(a.Domain)
	bDomain := normalizeDomain(b.Domain)
	return aDomain == bDomain &&
		a.Basepath == b.Basepath &&
		a.BackendBaseUrl == b.BackendBaseUrl
}

func (bc *BackendConfig) Prefix() string {
	if bc.MatchesAllDomains() {
		return bc.Basepath
	}

	// TODO we need the TLD from somewhere else
	// maybe needs to be added to the domain?
	tld := ".wip"
	return bc.Domain + tld + bc.Basepath
}

/**************************************
*** BackendConfigList starts here   ***
**************************************/
func (c *Config) AppendBackendConfig(conf BackendConfig) {
	fmt.Printf("append backend %#v to %#v\n", conf, c.backends)
	c.backends = append(c.backends, &conf)
	fmt.Printf("backend appended %#v\n", c.backends)
}

func (c *Config) RemoveBackendConfig(conf BackendConfig) error {
	fmt.Printf("remove backend %#v from %#v\n", conf, c.backends)
	// to not introduce subtle bugs we duplicate the elements, except the one which is equal
	// the list should be quite short, so no performance optimizations necessary
	backends := slices.DeleteFunc(c.backends, func(bc *BackendConfig) bool {
		return conf.Equals(bc)
	})
	fmt.Printf("backend removed %#v\n", c.backends)
	if len(backends) == len(c.backends) {
		return fmt.Errorf("nothing changed")
	}
	c.backends = backends
	return nil
}

func (bcList BackendConfigList) FindBackendConfigMatch(req *http.Request) *BackendConfig {
	for _, bc := range bcList {
		if bc.MatchHttpRequest(req) {
			return bc
		}
	}

	return nil
}

func (bc *BackendConfig) MatchHttpRequest(req *http.Request) bool {
	prefix := bc.Prefix()
	return strings.HasPrefix(req.URL.Path, prefix) ||
		strings.HasPrefix(req.Host+req.URL.Path, prefix)
}

func (bc *BackendConfig) RewriteRequestPath(requestPath string) string {
	withoutPrefix, found := strings.CutPrefix(requestPath, bc.Basepath)
	if !found {
		return requestPath
	}

	return bc.BackendBaseUrl + withoutPrefix
}

func (bc *BackendConfig) RewriteRequest(req *http.Request) (*http.Request, *http.Response) {
	urlString := bc.RewriteRequestPath(req.URL.Path)

	url, err := url.Parse(urlString)
	if err != nil {
		// something went wrong and urlString is not a valid url
		return req, &http.Response{StatusCode: http.StatusInternalServerError}
	}
	req.Host = url.Host
	req.URL = url
	req.Header.Add("X-Via", "symfony-cli")
	return req, nil
}
