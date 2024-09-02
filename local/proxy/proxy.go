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
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/elazarl/goproxy"
	"github.com/pkg/errors"
	"github.com/symfony-cli/cert"
	"github.com/symfony-cli/symfony-cli/local/html"
	"github.com/symfony-cli/symfony-cli/local/pid"
	"github.com/symfony-cli/symfony-cli/local/projects"
)

type Proxy struct {
	*Config
	proxy *goproxy.ProxyHttpServer
}

type ProxyRequest struct {
	backendHost string
	ipAndPort   string
	*http.Request
}

func NewProxyRequest(backendHost string, ipAndPort string, reader *bufio.Reader) (*ProxyRequest, error) {
	req, err := http.ReadRequest(reader)
	if err != nil {
		return nil, err
	}
	return &ProxyRequest{backendHost, ipAndPort, req}, nil
}

func close(c io.Closer) {
	if c != nil {
		c.Close()
	}
}

// only use this for prototyping
// func orPanic(err error) {
// 	if err != nil {
// 		fmt.Println("lets panic", err)
// 		panic(err)
// 	}
// }

func requestShouldGoToBackend(req *http.Request, bc BackendConfig) bool {
	return strings.HasPrefix(req.URL.Path, bc.Prefix()) ||
		strings.HasPrefix(req.Host+req.URL.Path, bc.Prefix())
}

func printProxyReq(prefix string, req *ProxyRequest, ctx *goproxy.ProxyCtx) {
	ctx.Warnf("%s req: %#v\n", prefix, req)
	ctx.Warnf("%s req: %#v\n", prefix, req.Request)
	ctx.Warnf("%s req.Schema: %#v\n", prefix, req.URL.Scheme)
	ctx.Warnf("%s req.Method: %#v\n", prefix, req.Method)
	ctx.Warnf("%s req.RequestURI: %#v\n", prefix, req.RequestURI)
	ctx.Warnf("%s req.URL: %#v\n", prefix, req.URL)
	ctx.Warnf("%s req.URL.RawPath: %#v\n", prefix, req.URL.RawPath)
}

func printReq(prefix string, req *http.Request, ctx *goproxy.ProxyCtx) {
	printProxyReq(prefix, &ProxyRequest{"", "", req}, ctx)
}

func getIpForDomain(domain string, ctx *goproxy.ProxyCtx) (net.IP, error) {
	backendIPs, err := net.LookupIP(domain)
	if err != nil {
		ctx.Warnf("net.LookupIP(%s): ", domain, err)
		return nil, err
	}
	for _, ip := range backendIPs {
		if ipv4 := ip.To4(); ipv4 != nil {
			ctx.Warnf("IPv4 for: %s\n", ipv4)
			return ipv4, nil
		}
		// TODO build IPv6 path
	}
	return nil, errors.New("Could not find an IP4")
}

func getPortForRequest(req *ProxyRequest) string {
	port := req.URL.Port()
	if port == "" {
		if req.URL.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return port
}

func (p *ProxyRequest) setIpAndPort(req *ProxyRequest, ctx *goproxy.ProxyCtx) error {
	ip, err := getIpForDomain(req.Host, ctx)
	if err != nil {
		return err
	}
	port := getPortForRequest(req)

	p.ipAndPort = fmt.Sprintf("%s:%s", ip, port)
	return nil
}

func createTargetTlsConfig(domain string, proxyClientTlsConfig *tls.Config, proxyClientTls *tls.Conn) *tls.Config {
	negotiatedProtocol := proxyClientTls.ConnectionState().NegotiatedProtocol
	if negotiatedProtocol == "" {
		negotiatedProtocol = "http/1.1"
	}

	// TODO: for wip domains use the original TLS config,
	// for everything else use default
	var rootCAs *x509.CertPool
	if domain == "localhost" {
		rootCAs = proxyClientTlsConfig.RootCAs
	}

	targetTlsConfig := &tls.Config{
		RootCAs:    rootCAs,
		ServerName: domain,
		NextProtos: []string{negotiatedProtocol},
	}

	return targetTlsConfig
}

func createBackendConnection(req *ProxyRequest, proxyClientTls *tls.Conn, proxyClientTlsConfig *tls.Config, targetSiteConn net.Conn, ctx *goproxy.ProxyCtx) (net.Conn, error) {

	targetTlsConfig := createTargetTlsConfig(req.backendHost, proxyClientTlsConfig, proxyClientTls)

	if req.URL.Scheme == "https" {
		targetSiteTlsConn := tls.Client(targetSiteConn, targetTlsConfig)

		if err := targetSiteTlsConn.Handshake(); err != nil {
			ctx.Warnf("Cannot handshake target %v %v", req.Host, err)
			badGatewayResponse(proxyClientTls, ctx, err)
			return nil, err
		}
		return targetSiteTlsConn, nil
	}
	return targetSiteConn, nil
}

func badGatewayResponse(w io.WriteCloser, ctx *goproxy.ProxyCtx, err error) {
	if _, err := io.WriteString(w, "HTTP/1.1 502 Bad Gateway\r\n\r\n"); err != nil {
		ctx.Warnf("Error responding to client: %s", err)
	}
	if err := w.Close(); err != nil {
		ctx.Warnf("Error closing client connection: %s", err)
	}
}

func tlsToLocalWebServer(proxy *goproxy.ProxyHttpServer, proxyClientTlsConfig *tls.Config, config *Config, backend string) *goproxy.ConnectAction {

	notImplementedResponse := func(w io.WriteCloser, ctx *goproxy.ProxyCtx, err error) {
		if _, err := io.WriteString(w, "HTTP/1.1 501 Not Implemented\r\n\r\n"); err != nil {
			ctx.Warnf("Error responding to client: %s", err)
		}
		// do not close the connection after sending this response, client may downgrade to HTTP/1.1
	}
	connectDial := func(proxy *goproxy.ProxyHttpServer, network, addr string) (c net.Conn, err error) {
		if proxy.ConnectDial != nil {
			return proxy.ConnectDial(network, addr)
		}
		if proxy.Tr.Dial != nil {
			return proxy.Tr.Dial(network, addr)
		}
		return net.Dial(network, addr)
	}
	// tlsRecordHeaderLooksLikeHTTP reports whether a TLS record header
	// looks like it might've been a misdirected plaintext HTTP request.
	tlsRecordHeaderLooksLikeHTTP := func(hdr [5]byte) bool {
		switch string(hdr[:]) {
		// YES this looks wrong. It's actually called OPTIONS,
		// but the reason is that there are only 5 bytes in the tls.RecordHeaderError.RecordHeader
		case "GET /", "HEAD ", "POST ", "PUT /", "OPTIO":
			return true
		}
		return false
	}
	return &goproxy.ConnectAction{
		Action: goproxy.ConnectHijack,
		Hijack: func(req *http.Request, proxyClient net.Conn, ctx *goproxy.ProxyCtx) {
			ctx.Warnf("Hijacking CONNECT")
			ctx.Warnf("HTTP method=%s\n", req.Method)

			// TODO implement HTTP/2.0 connections
			proxyClient.Write([]byte("HTTP/1.0 200 OK\r\n\r\n"))

			proxyClientTls := tls.Server(proxyClient, proxyClientTlsConfig)
			defer close(proxyClient)

			if err := proxyClientTls.Handshake(); err != nil {
				rhErr, ok := err.(tls.RecordHeaderError)
				if ok && rhErr.Conn != nil && tlsRecordHeaderLooksLikeHTTP(rhErr.RecordHeader) {
					io.WriteString(proxyClient, "HTTP/1.0 400 Bad Request\r\n\r\nClient sent an HTTP request to an HTTPS server.\n")
					return
				}

				ctx.Logf("TLS handshake error from %s: %v", proxyClient.RemoteAddr(), err)
				return
			}

			ctx.Warnf("Assuming CONNECT is TLS, TLS proxying it")
			printReq("Hijack req:", req, ctx)

			clientTlsReader := bufio.NewReader(proxyClientTls)
			clientTlsWriter := bufio.NewWriter(proxyClientTls)
			clientBuf := bufio.NewReadWriter(clientTlsReader, clientTlsWriter)
			myReq, err := NewProxyRequest("localhost", backend, clientBuf.Reader)
			if err != nil {
				ctx.Warnf("Problem reading from clientBuf.Reader %#v: %v\n", clientBuf.Reader, err)
			}

			myReq.URL.Scheme = "https" // every localhost request here has https

			for _, bc := range config.backends {
				ctx.Warnf("try to match prefix: myReq.Host='%s', myReq.URL.Path='%s', prefix='%s'",
					myReq.Host, myReq.URL.Path, bc.Prefix())

				if requestShouldGoToBackend(myReq.Request, bc) {
					ctx.Warnf("Hijack prefix matches")
					ctx.Warnf("myReq.URL.Path: %#v\n", myReq.URL.Path)
					urlString := bc.regexp.ReplaceAllLiteralString(myReq.URL.Path, bc.BackendBaseUrl)
					ctx.Warnf("urlstring: %#v\n", urlString)

					url, _ := url.Parse(urlString)
					// if err != nil {
					// 	// something went wrong and urlString is not a valid url
					// 	return myReq, &http.Response{StatusCode: http.StatusInternalServerError}
					// }
					myReq.Host = url.Hostname()
					myReq.backendHost = url.Hostname()
					myReq.URL = url
					myReq.RequestURI = ""
					myReq.Header.Add("X-Via", "symfony-cli")

					// lookup IP for Host
					err = myReq.setIpAndPort(myReq, ctx)
					if err != nil {
						return
					}

					break // we already found a match
				} else {
					ctx.Warnf("Hijack prefix didn't match")
				}
			}

			printProxyReq("Hijack myReq:", myReq, ctx)

			if myReq.Method == "PRI" {
				ctx.Warnf("This is a PRI request for HTTP/2.0, we don't serve HTTP/2.0 yet")
				notImplementedResponse(proxyClientTls, ctx, err)
				_, err := clientTlsReader.Discard(6)
				if err != nil {
					ctx.Warnf("Failed to process HTTP2 client preface: %v", err)
					return
				}
				return
			}

			// TODO find out why this is proxying on OSI layer 4 (tcp) and not OSI layer 7 (http)
			// TODO we need to implement a proxy on OSI layer 7,
			// so we can read the URI and proxy to the correct backend
			targetSiteConn, err := connectDial(proxy, "tcp", myReq.ipAndPort)
			defer close(targetSiteConn)

			if err != nil {
				ctx.Warnf(`Error for connectDial(proxy, "tcp", actualBackend) = %#v\n`, err)
				badGatewayResponse(proxyClientTls, ctx, err)
				return
			}

			backendConn, nil := createBackendConnection(myReq, proxyClientTls, proxyClientTlsConfig, targetSiteConn, ctx)
			defer close(backendConn)

			remoteBuf := bufio.NewReadWriter(bufio.NewReader(backendConn), bufio.NewWriter(backendConn))

			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				// proxy from client to backend
				err := myReq.Write(remoteBuf)
				if err != nil {
					ctx.Warnf("Error when calling myReq.Write(remoteBuf), myReq=%#v, remoteBuf=%#v: %v\n", myReq, remoteBuf, err)
				}

				err = remoteBuf.Flush()
				if err != nil {
					ctx.Warnf("Error when calling remoteBuf.Flush(), remoteBuf=%#v: %v\n", remoteBuf, err)
				}

				err = myReq.Body.Close()
				if err != nil {
					ctx.Warnf("Error with myReq.Body.Close(), myReq.Body=%#v: %v\n", myReq.Body, err)
				}

				wg.Done()
			}()

			go func() {
				// proxy from backend to client
				resp, err := http.ReadResponse(remoteBuf.Reader, myReq.Request)
				if err != nil {
					ctx.Warnf("Problem with http.ReadResponse, remoteBuf.Reader=%#v: %v\n", remoteBuf.Reader, err)
				}

				err = resp.Write(clientBuf.Writer)
				if err != nil {
					ctx.Warnf("Problem with resp.Write, clientBuf.Writer=%#v: %v\n", clientBuf.Writer, err)
				}

				err = resp.Body.Close()
				if err != nil {
					ctx.Warnf("Problem with resp.Body.Close(), resp.Body=%#v: %v\n", resp.Body, err)
				}

				err = clientBuf.Flush()
				if err != nil {
					ctx.Warnf("Problem with clientBuf.Flush(), clientBuf=%#v: %v\n", clientBuf, err)
				}

				wg.Done()
			}()
			wg.Wait()
		},
	}
}

func New(config *Config, ca *cert.CA, logger *log.Logger, debug bool) *Proxy {
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = debug
	proxy.Logger = logger
	p := &Proxy{
		Config: config,
		proxy:  proxy,
	}

	var proxyTLSConfig *tls.Config

	if ca != nil {
		goproxy.GoproxyCa = *ca.AsTLS()
		getCertificate := p.newCertStore(ca).getCertificate
		cert, err := x509.ParseCertificate(ca.AsTLS().Certificate[0])
		if err != nil {
			panic(err)
		}
		certpool := x509.NewCertPool()
		certpool.AddCert(cert)
		tlsConfig := &tls.Config{
			RootCAs:        certpool,
			GetCertificate: getCertificate,
			NextProtos:     []string{"http/1.1", "http/1.0"},
		}
		proxyTLSConfig = &tls.Config{
			RootCAs:        certpool,
			GetCertificate: getCertificate,
			NextProtos:     []string{"http/1.1", "h2", "http/1.0"},
		}
		tlsConfigFunc := func(host string, ctx *goproxy.ProxyCtx) (*tls.Config, error) {
			return tlsConfig, nil
		}
		// They don't use TLSConfig but let's keep them in sync
		goproxy.MitmConnect.TLSConfig = tlsConfigFunc
		goproxy.OkConnect.TLSConfig = tlsConfigFunc
		goproxy.RejectConnect.TLSConfig = tlsConfigFunc
		goproxy.HTTPMitmConnect.TLSConfig = tlsConfigFunc
	}
	proxy.NonproxyHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "" {
			fmt.Fprintln(w, "Cannot handle requests without a Host header, e.g. HTTP 1.0")
			return
		}
		r.URL.Scheme = "http"
		r.URL.Host = r.Host
		if r.URL.Path == "/proxy.pac" {
			p.servePacFile(w, r)
			return
		} else if r.URL.Path == "/" {
			p.serveIndex(w, r)
			return
		}
		http.Error(w, "Not Found", 404)
	})

	// proxy only where the TLD matches
	cond := proxy.OnRequest(config.tldMatches())
	cond.HandleConnectFunc(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		hostName, hostPort, err := net.SplitHostPort(host)
		if err != nil {
			// probably because no port in the host (determine it via the scheme)
			if ctx.Req.URL.Scheme == "https" {
				hostPort = "443"
			} else {
				hostPort = "80"
			}
			hostName = ctx.Req.Host
		}
		// wrong port for scheme?
		if ctx.Req.URL.Scheme == "https" && hostPort != "443" {
			return goproxy.MitmConnect, host
		} else if ctx.Req.URL.Scheme == "http" && hostPort != "80" {
			return goproxy.MitmConnect, host
		}

		req := ctx.Req

		printReq("HandleConnectFunc:", req, ctx)

		projectDir := p.GetDir(hostName)
		if projectDir == "" {
			return goproxy.MitmConnect, host
		}

		pid := pid.New(projectDir, nil)
		if !pid.IsRunning() {
			return goproxy.MitmConnect, host
		}

		backend := fmt.Sprintf("127.0.0.1:%d", pid.Port)

		if hostPort != "443" {
			// No TLS termination required, let's go through regular proxy
			return goproxy.OkConnect, backend
		}

		if proxyTLSConfig != nil {
			// the request came via HTTPS
			// return goproxy.OkConnect, backend
			return tlsToLocalWebServer(proxy, proxyTLSConfig, p.Config, backend), backend
		}

		// We didn't manage to get a tls.Config, we can't fulfill this request hijacking TLS
		return goproxy.RejectConnect, backend
	})

	cond.DoFunc(func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		req := ctx.Req
		printReq("DoFunc:", req, ctx)

		for _, bc := range config.backends {
			ctx.Warnf("prefix: %s", bc.Prefix())

			if requestShouldGoToBackend(req, bc) {

				ctx.Warnf("DoFunc prefix matches")

				urlString := bc.Regexp().ReplaceAllLiteralString(req.URL.Path, bc.BackendBaseUrl)
				url, err := url.Parse(urlString)
				if err != nil {
					// something went wrong and urlString is not a valid url
					return req, &http.Response{StatusCode: http.StatusInternalServerError}
				}
				req.Host = url.Host
				req.URL = url
				req.Header.Add("X-Via", "symfony-cli")
				return req, nil
			} else {
				ctx.Warnf("DoFunc prefix didn't match")
			}
		}

		hostName, hostPort, err := net.SplitHostPort(r.Host)
		if err != nil {
			// probably because no port in the host (determine it via the scheme)
			if r.URL.Scheme == "https" {
				hostPort = "443"
			} else {
				hostPort = "80"
			}
			hostName = r.Host
		}
		// wrong port?
		if r.URL.Scheme == "https" && hostPort != "443" {
			return r, goproxy.NewResponse(r,
				goproxy.ContentTypeHtml, http.StatusNotFound,
				html.WrapHTML(
					"Proxy Error",
					html.CreateErrorTerminal(`You must use port 443 for HTTPS requests (%s used)`, hostPort)+
						html.CreateAction(fmt.Sprintf("https://%s/", hostName), "Go to port 443"), ""),
			)
		} else if r.URL.Scheme == "http" && hostPort != "80" {
			return r, goproxy.NewResponse(r,
				goproxy.ContentTypeHtml, http.StatusNotFound,
				html.WrapHTML(
					"Proxy Error",
					html.CreateErrorTerminal(`You must use port 80 for HTTP requests (%s used)`, hostPort)+
						html.CreateAction(fmt.Sprintf("http://%s/", hostName), "Go to port 80"), ""),
			)
		}
		projectDir := p.GetDir(hostName)
		if projectDir == "" {
			hostNameWithoutTLD := strings.TrimSuffix(hostName, "."+p.TLD)
			hostNameWithoutTLD = strings.TrimPrefix(hostNameWithoutTLD, "www.")

			// the domain does not refer to any project
			return r, goproxy.NewResponse(r,
				goproxy.ContentTypeHtml, http.StatusNotFound,
				html.WrapHTML("Proxy Error", html.CreateErrorTerminal(`# The "%s" hostname is not linked to a directory yet.
# Link it via the following command:

<code>symfony proxy:domain:attach %s --dir=/some/dir</code>`, hostName, hostNameWithoutTLD), ""))
		}

		pid := pid.New(projectDir, nil)
		if !pid.IsRunning() {
			return r, goproxy.NewResponse(r,
				goproxy.ContentTypeHtml, http.StatusNotFound,
				// colors from http://ethanschoonover.com/solarized
				html.WrapHTML(
					"Proxy Error",
					html.CreateErrorTerminal(`# It looks like the web server associated with the "%s" hostname is not started yet.
# Start it via the following command:

$ symfony server:start --daemon --dir=%s`,
						hostName, projectDir)+
						html.CreateAction("", "Retry"), ""),
			)
		}

		r.URL.Host = fmt.Sprintf("127.0.0.1:%d", pid.Port)

		if r.Header.Get("X-Forwarded-Port") == "" {
			r.Header.Set("X-Forwarded-Port", hostPort)
		}

		return r, nil
	})
	return p
}

func (p *Proxy) Start() error {
	go p.Config.Watch()
	return errors.WithStack(http.ListenAndServe(":"+strconv.Itoa(p.Port), p.proxy))
}

func (p *Proxy) servePacFile(w http.ResponseWriter, r *http.Request) {
	// Use the current request hostname (r.Host) to generate the PAC file.
	// This means that as soon as you are able to reach the proxy, the generated
	// PAC file will expose an appropriate hostname or IP even if the proxy
	// is running remotely, in a container or a VM.
	// No need to fall back to p.Host and p.Port as r.Host is already checked
	// upper in the stacktrace.
	w.Header().Add("Content-Type", "application/x-ns-proxy-autoconfig")
	w.Write([]byte(fmt.Sprintf(`// Only proxy *.%s requests
// Configuration file in ~/.symfony5/proxy.json
function FindProxyForURL (url, host) {
	if (dnsDomainIs(host, '.%s')) {
		if (isResolvable(host)) {
			return 'DIRECT';
		}

		return 'PROXY %s';
	}

	return 'DIRECT';
}
`, p.TLD, p.TLD, r.Host)))
}

func (p *Proxy) serveIndex(w http.ResponseWriter, r *http.Request) {
	content := ``

	proxyProjects, err := ToConfiguredProjects()
	if err != nil {
		return
	}
	runningProjects, err := pid.ToConfiguredProjects(true)
	if err != nil {
		return
	}
	projects, err := projects.GetConfiguredAndRunning(proxyProjects, runningProjects)
	if err != nil {
		return
	}
	projectDirs := []string{}
	for dir := range projects {
		projectDirs = append(projectDirs, dir)
	}
	sort.Strings(projectDirs)

	content += "<table><tr><th>Directory<th>Port<th>Domains"
	for _, dir := range projectDirs {
		project := projects[dir]
		content += fmt.Sprintf("<tr><td>%s", dir)
		if project.Port > 0 {
			content += fmt.Sprintf(`<td><a href="http://127.0.0.1:%d/">%d</a>`, project.Port, project.Port)
		} else {
			content += `<td style="color: #b58900">Not running`
		}
		content += "<td>"
		for _, domain := range project.Domains {
			if strings.Contains(domain, "*") {
				content += fmt.Sprintf(`%s://%s/`, project.Scheme, domain)
			} else {
				content += fmt.Sprintf(`<a href="%s://%s/">%s://%s/</a>`, project.Scheme, domain, project.Scheme, domain)
			}
			content += "<br>"
		}
	}
	w.Write([]byte(html.WrapHTML("Proxy Index", html.CreateTerminal(content), "")))
}
