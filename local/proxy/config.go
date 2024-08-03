/*
 * Copyright (c) 2021-present Fabien Potencier <fabien@symfony.com>
 *
 * This file is part of Symfony CLI project
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as
 * published by the Free Software Foundation, either version 3 of the
 * License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program. If not, see <http://www.gnu.org/licenses/>.
 */

package proxy

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/elazarl/goproxy"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/symfony-cli/symfony-cli/inotify"
	"github.com/symfony-cli/symfony-cli/local/projects"
	"github.com/symfony-cli/symfony-cli/util"
)

type Config struct {
	TLD  string `json:"tld"`
	Host string `json:"host"`
	Port int    `json:"port"`
	// only here so that we can unmarshal :(
	TmpDomains  map[string]string `json:"domains"`
	TmpBackends BackendConfigList `json:"backends"`
	path        string

	mu       sync.RWMutex
	domains  map[string]string
	backends BackendConfigList
}


var DefaultConfig = []byte(`{
	"tld": "wip",
	"host": "localhost",
	"port": 7080,
	"domains": {},
	"backends": []
}
`)

// TODO maybe use io.Reader, so we can pass the file or something else (for tests)
func Load(homeDir string) (*Config, error) {
	proxyFile := filepath.Join(homeDir, "proxy.json")
	if _, err := os.Stat(proxyFile); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(proxyFile), 0755); err != nil {
			return nil, errors.Wrapf(err, "unable to create directory for %s", proxyFile)
		}
		if err := os.WriteFile(proxyFile, DefaultConfig, 0644); err != nil {
			return nil, errors.Wrapf(err, "unable to write %s", proxyFile)
		}
	}
	data, err := os.ReadFile(proxyFile)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read the proxy configuration file, %s", proxyFile)
	}
	var config *Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, errors.Wrapf(err, "unable to parse the JSON proxy configuration file, %s", proxyFile)
	}
	if config.Host == "" {
		config.Host = "localhost"
	}
	if config.TmpDomains == nil {
		// happens if one has removed the domains manually in the file
		config.domains = make(map[string]string)
	} else {
		config.SetDomains(config.TmpDomains)
		config.TmpDomains = nil
	}
	if (reflect.DeepEqual(config.TmpBackends, BackendConfigList{})) {
		// happens if one has removed the backends manually in the file
		// or when we upgrade
		config.backends = BackendConfigList{}
	} else {
		config.SetBackends(config.TmpBackends)
		config.TmpBackends = BackendConfigList{}
	}
	config.path = proxyFile
	return config, nil
}

func ToConfiguredProjects() (map[string]*projects.ConfiguredProject, error) {
	ps := make(map[string]*projects.ConfiguredProject)
	userHomeDir, err := homedir.Dir()
	if err != nil {
		userHomeDir = ""
	}

	homeDir := util.GetHomeDir()
	proxyConf, err := Load(homeDir)
	if err != nil {
		return nil, err
	}
	dirs := proxyConf.Dirs()
	for dir := range dirs {
		shortDir := dir
		if strings.HasPrefix(dir, userHomeDir) {
			shortDir = "~" + dir[len(userHomeDir):]
		}

		ps[shortDir] = &projects.ConfiguredProject{
			Domains: proxyConf.GetDomains(dir),
			Scheme:  "https",
		}
	}
	return ps, nil
}

func GetConfig() (*Config, error) {
	homeDir := util.GetHomeDir()
	proxyConf, err := Load(homeDir)
	if err != nil {
		return nil, err
	}
	return proxyConf, nil
}

func (c *Config) Domains() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.domains
}

func (c *Config) Dirs() map[string][]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	dirs := map[string][]string{}
	for dir, domain := range c.domains {
		dirs[domain] = append(dirs[domain], dir)
	}
	return dirs
}

func (c *Config) NormalizeDomain(domain string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.doNormalizeDomain(domain)
}

func (c *Config) GetDir(domain string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.domains[c.domainWithoutTLD(c.doNormalizeDomain(domain))]
}

func (c *Config) GetDomains(dir string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	domains := []string{}
	for domain, d := range c.domains {
		if d == dir {
			domains = append(domains, domain+"."+c.TLD)
		}
	}
	return domains
}

// TODO do we need to implement GetBackends() ? And how should it look like?

func (c *Config) GetBackends() BackendConfigList {
	return c.backends
}


func (c *Config) GetReachableDomains(dir string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	domains := []string{}
	for domain, d := range c.domains {
		// domain is defined using a wildcard: we don't know the exact domain,
		// so we can't use it directly as-is to reach the project
		if strings.Contains(domain, "*") {
			continue
		}
		if d == dir {
			domains = append(domains, domain+"."+c.TLD)
		}
	}
	return domains
}

func (c *Config) SetDomains(domains map[string]string) {
	c.mu.Lock()
	c.domains = domains
	c.mu.Unlock()
}

func (c *Config) SetBackends(backends BackendConfigList) {
	c.mu.Lock()
	c.backends = backends
	c.mu.Unlock()
}

func (c *Config) ReplaceDirDomains(dir string, domains []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for domain, d := range c.domains {
		if d == dir {
			delete(c.domains, domain)
		}
	}
	for _, d := range domains {
		if strings.HasSuffix(d, c.TLD) {
			return errors.Errorf(`domain "%s" must not end with the "%s" TLD, please remove the TLD`, d, c.TLD)
		}
		c.domains[d] = dir
	}
	return c.Save()
}

func (c *Config) AddDirDomains(dir string, domains []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, d := range domains {
		if strings.HasSuffix(d, c.TLD) {
			return errors.Errorf(`domain "%s" must not end with the "%s" TLD, please remove the TLD`, d, c.TLD)
		}
		c.domains[d] = dir
	}
	return c.Save()
}

func (c *Config) RemoveDirDomains(domains []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, d := range domains {
		if strings.HasSuffix(d, c.TLD) {
			return errors.Errorf(`domain "%s" must not end with the "%s" TLD, please remove the TLD`, d, c.TLD)
		}
		delete(c.domains, d)
	}
	return c.Save()
}

// Watch checks config file changes
func (c *Config) Watch() {
	watcherChan := make(chan inotify.EventInfo, 1)
	if err := inotify.Watch(c.path, watcherChan, inotify.Write); err != nil {
		log.Printf("unable to watch proxy config file: %s", err)
	}
	defer inotify.Stop(watcherChan)
	for {
		<-watcherChan
		c.reload()
	}
}

// reloads the TLD and the domains (not the port)
// TODO it would be nice if this would use the same loading code as the default config
// just by using an io.Reader interface which is already implemented for files
func (c *Config) reload() {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return
	}
	c.SetDomains(config.TmpDomains)
	c.SetBackends(config.TmpBackends)
	c.mu.Lock()
	c.TLD = config.TLD
	c.mu.Unlock()
}

func (c *Config) tldMatches() goproxy.ReqConditionFunc {
	re := regexp.MustCompile(fmt.Sprintf("\\.%s(\\:\\d+)?$", c.TLD))

	return func(req *http.Request, ctx *goproxy.ProxyCtx) bool {
		return re.MatchString(req.Host)
	}
}

func (c *Config) Save() error {
	// TODO not sure why this is the case here
	// my guess is, there should be two different kind of structs, which look similar
	// but one is for the internal domain and one is for marshalling / unmarshalling
	c.TmpDomains = c.domains
	c.TmpBackends = c.backends
	data, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(os.WriteFile(c.path, data, 0644))
}

// should be called with a lock a place
// always returns a domain with the TLD
func (c *Config) doNormalizeDomain(domain string) string {
	domain = c.domainWithoutTLD(domain)
	fqdn := domain + "." + c.TLD
	if _, ok := c.domains[domain]; ok {
		return fqdn
	}
	match := ""
	for d := range c.domains {
		if !strings.Contains(d, "*") {
			continue
		}
		// glob matching
		if strings.HasSuffix(domain, strings.Replace(d, "*.", ".", -1)) {
			m := d + "." + c.TLD
			// always use the longest possible domain for matching
			if len(m) > len(match) {
				match = m
			}
		}
	}
	if match != "" {
		return match
	}
	return fqdn
}

func (c *Config) domainWithoutTLD(domain string) string {
	if strings.HasSuffix(domain, "."+c.TLD) {
		return domain[:len(domain)-len(c.TLD)-1]
	}
	return domain
}
