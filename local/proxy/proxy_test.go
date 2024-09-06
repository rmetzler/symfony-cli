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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/rs/zerolog"
	"github.com/symfony-cli/cert"
	"github.com/symfony-cli/symfony-cli/local/pid"
	. "gopkg.in/check.v1"
)

type dummyPhpBackend struct {
	cmd *exec.Cmd
}

func start(projectDir string, port int) dummyPhpBackend {
	p := pid.New(projectDir, nil)
	cmd := exec.Command("sleep", "5")
	cmd.Start()
	p.Write(cmd.Process.Pid, port, "http")
	return dummyPhpBackend{cmd}

}

func (d *dummyPhpBackend) pid() int {
	return d.cmd.Process.Pid
}

func (d *dummyPhpBackend) stop() {
	syscall.Kill(d.cmd.Process.Pid, syscall.SIGTERM)
	d.cmd.Wait()
}

func (s *ProxySuite) TestProxy(c *C) {
	ca, err := cert.NewCA(filepath.Join("testdata/certs"))
	c.Assert(err, IsNil)
	c.Assert(ca.LoadCA(), IsNil)

	homedir.Reset()
	os.Setenv("HOME", "testdata")
	defer homedir.Reset()
	defer os.RemoveAll("testdata/.symfony5")

	generalBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`general backend`))
	}))
	defer generalBackend.Close()

	p := New(
		&Config{
			domains: map[string]string{
				"symfony":             "symfony_com",
				"symfony-not-started": "symfony_com_not_started",
				"symfony-no-tls":      "symfony_com_no_tls",
				"symfony2":            "symfony_com2",
			},
			TLD:  "wip",
			path: "testdata/.symfony5/proxy.json",
			backends: BackendConfigList{
				&BackendConfig{Domain: "*", Basepath: "/star", BackendBaseUrl: generalBackend.URL},
			},
		},
		ca,
		log.New(zerolog.New(os.Stderr), "", 0),
		true,
	)
	os.MkdirAll("testdata/.symfony5", 0755)
	err = p.Save()
	c.Assert(err, IsNil)

	// Test the 404 fallback
	{
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/foo", nil)
		req.Host = "localhost"
		c.Assert(err, IsNil)
		p.proxy.ServeHTTP(rr, req)
		c.Check(rr.Code, Equals, http.StatusNotFound)
	}

	// Test serving the proxy.pac
	{
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/proxy.pac", nil)
		req.Host = "localhost"
		c.Assert(err, IsNil)
		p.proxy.ServeHTTP(rr, req)
		c.Assert(rr.Code, Equals, http.StatusOK)
		c.Check(rr.Header().Get("Content-type"), Equals, "application/x-ns-proxy-autoconfig")
	}

	// Test serving the index
	{
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/", nil)
		req.Host = "localhost"
		c.Assert(err, IsNil)
		p.proxy.ServeHTTP(rr, req)
		c.Assert(rr.Code, Equals, http.StatusOK)
		c.Check(strings.Contains(rr.Body.String(), "symfony.wip"), Equals, true)
	}

	// Test the proxy
	frontend := httptest.NewServer(p.proxy)
	defer frontend.Close()
	frontendUrl, _ := url.Parse(frontend.URL)
	cert, err := x509.ParseCertificate(ca.AsTLS().Certificate[0])
	c.Assert(err, IsNil)
	certpool := x509.NewCertPool()
	certpool.AddCert(cert)
	transport := &http.Transport{
		Proxy: http.ProxyURL(frontendUrl),
		TLSClientConfig: &tls.Config{
			RootCAs: certpool,
		},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   1 * time.Second,
	}

	// Test proxying a request to a non-registered project
	{
		fmt.Printf("\nKMD Test foo\n")
		req, _ := http.NewRequest("GET", "https://foo.wip/", nil)
		req.Close = true

		res, err := client.Do(req)
		c.Assert(err, IsNil)
		c.Assert(res.StatusCode, Equals, http.StatusNotFound)
		body, _ := io.ReadAll(res.Body)
		c.Check(strings.Contains(string(body), "not linked"), Equals, true)
	}

	// Test proxying a request to a registered project but not started
	{
		fmt.Printf("\nKMD Test symfony not running\n")
		req, _ := http.NewRequest("GET", "https://symfony-not-started.wip/", nil)
		req.Close = true

		res, err := client.Do(req)
		c.Assert(err, IsNil)
		c.Assert(res.StatusCode, Equals, http.StatusNotFound)
		body, _ := io.ReadAll(res.Body)
		c.Check(strings.Contains(string(body), "not started"), Equals, true)
	}
	/*
		// Test proxying a request to a registered project and started
		{
			backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				w.Write([]byte(`symfony.wip`))
			}))
			cert, err := ca.CreateCert([]string{"localhost", "127.0.0.1"})
			c.Assert(err, IsNil)
			backend.TLS = &tls.Config{
				Certificates: []tls.Certificate{cert},
			}
			backend.StartTLS()
			defer backend.Close()
			backendURL, err := url.Parse(backend.URL)
			c.Assert(err, IsNil)

			p := pid.New("symfony_com", nil)
			port, _ := strconv.Atoi(backendURL.Port())
			p.Write(os.Getpid(), port, "https")

			req, _ := http.NewRequest("GET", "https://symfony.wip/", nil)
			req.Close = true

			res, err := client.Do(req)
			c.Assert(err, IsNil)
			c.Assert(res.StatusCode, Equals, http.StatusOK)
			body, _ := io.ReadAll(res.Body)
			c.Check(string(body), Equals, "symfony.wip")
		}
	*/
	// Test proxying a request to a registered project but no TLS
	{
		fmt.Printf("\nKMD Test symfony running without tls\n")
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte(`http://symfony-no-tls.wip`))
		}))
		defer backend.Close()
		backendURL, err := url.Parse(backend.URL)
		c.Assert(err, IsNil)

		port, _ := strconv.Atoi(backendURL.Port())
		backendProcess := start("symfony_com_no_tls", port)

		req, _ := http.NewRequest("GET", "http://symfony-no-tls.wip/", nil)
		req.Close = true

		res, err := client.Do(req)
		c.Assert(err, IsNil)
		body, _ := io.ReadAll(res.Body)
		c.Assert(res.StatusCode, Equals, http.StatusOK)
		c.Assert(string(body), Equals, "http://symfony-no-tls.wip")
		backendProcess.pid()

	}

	// Test proxying a request to an outside backend
	{
		fmt.Printf("\nKMD Test outside backend\n")
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
		}))
		defer backend.Close()
		req, _ := http.NewRequest("GET", backend.URL, nil)
		req.Close = true

		res, err := client.Do(req)
		c.Assert(err, IsNil)
		c.Assert(res.StatusCode, Equals, http.StatusOK)
	}
	// Test send request to the general http backend
	{
		fmt.Printf("\nKMD Test general http backend\n")
		for domain, _ := range p.Config.domains {
			req, _ := http.NewRequest("GET", fmt.Sprintf("http://%s.wip/star", domain), nil)
			req.Close = true

			res, err := client.Do(req)
			c.Assert(err, IsNil)
			body, _ := io.ReadAll(res.Body)
			c.Assert(res.StatusCode, Equals, http.StatusOK)
			c.Assert(string(body), Equals, "general backend")
		}
	}
	// Test send request to the general http backend for https call with running backend
	{
		fmt.Printf("\nKMD Test general https backend \n")
		domain := "symfony"
		backendProcess := start(p.Config.domains[domain], 0)

		req, _ := http.NewRequest("GET", fmt.Sprintf("https://%s.wip/star", domain), nil)
		req.Close = true

		res, err := client.Do(req)
		c.Assert(err, IsNil)
		body, _ := io.ReadAll(res.Body)
		c.Assert(res.StatusCode, Equals, http.StatusOK)
		c.Assert(string(body), Equals, "general backend")
		backendProcess.stop()
	}

	/*
		// Test proxying a request over HTTP2
		http2.ConfigureTransport(transport)
		{
			backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				if r.Proto == "HTTP/2.0" {
					w.Write([]byte(`http2`))
					return
				}
				w.Write([]byte(`symfony.wip`))
			}))
			cert, err := ca.CreateCert([]string{"localhost", "127.0.0.1"})
			c.Assert(err, IsNil)
			backend.TLS = &tls.Config{
				Certificates: []tls.Certificate{cert},
				NextProtos:   []string{"h2", "http/1.1"},
			}
			backend.StartTLS()
			defer backend.Close()
			backendURL, err := url.Parse(backend.URL)
			c.Assert(err, IsNil)

			p := pid.New("symfony_com2", nil)
			port, _ := strconv.Atoi(backendURL.Port())
			p.Write(os.Getpid(), port, "https")

			req, _ := http.NewRequest("GET", "https://symfony2.wip/", nil)
			req.Close = true

			res, err := client.Do(req)
			c.Assert(err, IsNil)
			c.Assert(res.StatusCode, Equals, http.StatusOK)
			body, _ := ioutil.ReadAll(res.Body)
			c.Check(string(body), Equals, "http2")
		}
	*/
}
