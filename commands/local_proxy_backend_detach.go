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

package commands

import (
	"fmt"

	"github.com/symfony-cli/console"
	"github.com/symfony-cli/symfony-cli/local/proxy"
	"github.com/symfony-cli/symfony-cli/util"
)



var localProxyDetachBackendCmd = &console.Command{
	Category: "local",
	Name:     "proxy:backend:detach",
	Aliases:  []*console.Alias{{Name: "proxy:backend:detach"}, {Name: "proxy:backend:remove", Hidden: true}},
	Usage:    "Detach backend from the proxy",
	Flags: []console.Flag{
		domainFlag,
		backendFlag,
		basepathFlag,
	},
	Action: func(c *console.Context) error {
		homeDir := util.GetHomeDir()
		config, err := proxy.Load(homeDir)
		if err != nil {
			return err
		}

		bc := proxy.BackendConfig{
			Domain:         c.String("domain"),
			BackendBaseUrl: c.String("backend"),
			Basepath:       c.String("basepath"),
		}

		err = config.RemoveBackendConfig(bc)
		if err != nil {
			fmt.Println(err)
		}

		err = config.Save()
		if err != nil {
			return err
		}

		return nil
	},
}
