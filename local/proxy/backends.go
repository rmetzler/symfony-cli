package proxy

import (
	"fmt"
	"regexp"
)

type BackendConfigList []BackendConfig

//	type BackendConfigList struct {
//		Default BackendConfig
//		backendConfigList []BackendConfig
//	}
type BackendConfig struct {
	Domain         string `json:"domain"   yaml:"domain"`
	Basepath       string `json:"basepath" yaml:"basepath"`
	BackendBaseUrl string `json:"backend"  yaml:"backend"`
	regexp         *regexp.Regexp
}

func (bc BackendConfig) Prefix() string {
	var prefix string
	if (bc.Domain == "") || (bc.Domain == "*") {
		prefix = bc.Basepath
	} else {
		// TODO we need the TLD from somewhere else
		tld := ".wip"
		prefix = bc.Domain + tld + bc.Basepath
	}
	return prefix
}

func (c *Config) AppendBackendConfig(conf BackendConfig) {
	fmt.Printf("append %#v %#v\n", c.backends, conf)
	c.backends = append(c.backends, conf)
	fmt.Printf("append %#v %#v\n", c.backends, conf)
}

func (bc *BackendConfig) Regexp() *regexp.Regexp {
	if bc.regexp == nil {
		bc.regexp = regexp.MustCompile(`^` + bc.Basepath)
	}
	return bc.regexp
}
