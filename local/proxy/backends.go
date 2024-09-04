package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

type BackendConfigList []*BackendConfig

type BackendConfig struct {
	Domain         string `json:"domain"   yaml:"domain"`
	Basepath       string `json:"basepath" yaml:"basepath"`
	BackendBaseUrl string `json:"backend"  yaml:"backend"`

	// TODO maybe not needed
	// regexp is lazily compiled from the Basepath
	// please do not call it directly, because the pointer can be nil
	// use Regexp() instead
	regexp *regexp.Regexp
}

func (a *BackendConfig) Equals(b *BackendConfig) bool {
	if a == b {
		return true
	}
	return a.Domain == b.Domain &&
		a.Basepath == b.Basepath &&
		a.BackendBaseUrl == b.BackendBaseUrl
}

func (bc BackendConfig) Prefix() string {
	var prefix string
	if (bc.Domain == "") || (bc.Domain == "*") {
		prefix = bc.Basepath
	} else {
		// TODO we need the TLD from somewhere else
		// maybe needs to be added to the domain?
		tld := ".wip"
		prefix = bc.Domain + tld + bc.Basepath
	}
	return prefix
}

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

// TODO probably not needed
// lazily compile the Regexp from the Basepath
func (bc *BackendConfig) Regexp() *regexp.Regexp {
	if bc.regexp == nil {
		bc.regexp = regexp.MustCompile(`^` + bc.Basepath)
	}
	return bc.regexp
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
	// urlString := bc.Regexp().ReplaceAllLiteralString(req.URL.Path, bc.BackendBaseUrl)
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
