/*
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

package commands

import (
	"github.com/symfony-cli/console"
	"github.com/symfony-cli/symfony-cli/local/proxy"
	"github.com/symfony-cli/symfony-cli/util"
	"github.com/symfony-cli/terminal"
)

var (
	backendFlag = &console.StringFlag{
		Name:  "backend",
		Usage: "proxy backend, complete with schema, port, domain and path",
	}
	basepathFlag = &console.StringFlag{
		Name:  "basepath",
		Usage: "basepath to be mounted in the proxy and be replaced with the backend",
	}
)

var localProxyAttachBackendCmd = &console.Command{
	Category: "local",
	Name:     "proxy:backend:attach",
	Aliases:  []*console.Alias{{Name: "proxy:backend:attach"}, {Name: "proxy:backend:add", Hidden: true}},
	Usage:    "Attach a backend under a basepath for the proxy",
	Flags: []console.Flag{
		dirFlag,
		backendFlag,
		basepathFlag,
	},
	Args: []*console.Arg{
		{Name: "domains", Optional: true, Description: "The project's domains", Slice: true},
	},
	Action: func(c *console.Context) error {
		projectDir, err := getProjectDir(c.String("dir"))
		if err != nil {
			return err
		}

		homeDir := util.GetHomeDir()
		config, err := proxy.Load(homeDir)
		if err != nil {
			return err
		}

		config.AppendBackendConfig(
			proxy.BackendConfig{
				Domain:         "*", // c.String("domains"),
				BackendBaseUrl: c.String("backend"),
				Basepath:       c.String("basepath"),
			})

		terminal.Println("<info>The proxy is now configured with the following domains for this directory:</>")

		for _, domain := range config.GetDomains(projectDir) {
			terminal.Printfln(" * http://%s", domain)
		}

		err = config.Save()
		if err != nil {
			return err
		}

		return nil
	},
}
