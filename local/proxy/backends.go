package proxy

import "fmt"

type BackendConfigList []BackendConfig

// type BackendConfigList struct {
// 	Default BackendConfig
// 	backendConfigList []BackendConfig
// }
type BackendConfig struct {
	Domain         string `json:"domain"   yaml:"domain"`
	Basepath       string `json:"basepath" yaml:"basepath"`
	BackendBaseUrl string `json:"backend"  yaml:"backend"`
}


func (c *Config) Append(conf BackendConfig)  {
	fmt.Printf("append %#v %#v\n", c.backends, conf)
	c.backends = append(c.backends, conf)
	fmt.Printf("append %#v %#v\n", c.backends, conf)
}
