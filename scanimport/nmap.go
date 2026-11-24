package scanimport

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

type nmapHost struct {
	Status struct {
		State string `xml:"state,attr"`
	} `xml:"status"`
	Addrs []struct {
		Addr string `xml:"addr,attr"`
		Type string `xml:"addrtype,attr"`
	} `xml:"address"`
	Names []struct {
		Name string `xml:"name,attr"`
		Type string `xml:"type,attr"`
	} `xml:"hostnames>hostname"`
	Ports []struct {
		Proto string `xml:"protocol,attr"`
		ID    string `xml:"portid,attr"`
		State struct {
			State string `xml:"state,attr"`
		} `xml:"state"`
		Svc struct {
			Name    string   `xml:"name,attr"`
			Product string   `xml:"product,attr"`
			Version string   `xml:"version,attr"`
			Extra   string   `xml:"extrainfo,attr"`
			Tunnel  string   `xml:"tunnel,attr"`
			CPE     []string `xml:"cpe"`
		} `xml:"service"`
	} `xml:"ports>port"`
	OS []struct {
		Name     string `xml:"name,attr"`
		Accuracy string `xml:"accuracy,attr"`
	} `xml:"os>osmatch"`
}

// parseNmap streams nmap -oX output one <host> at a time. Hosts that are up
// become an ip asset (with its open ports), a service or http_service asset
// per open port, and a subdomain asset per user-supplied hostname. The ip
// asset's Detail keeps the untrusted banners (product, version, CPE), every
// hostname and the OS guesses. Nmap yields no findings; <script> output is ignored. A file cut short keeps the hosts
// read before the cut and is marked truncated.
func parseNmap(c *collector, r io.Reader) error {
	dec := newXMLDecoder(r)
	root := false
	idx := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			if !root {
				return fmt.Errorf("scanimport: nmap-xml: not an nmap XML file: %w", err)
			}
			if !errors.Is(err, io.EOF) {
				c.res.Truncated = true
			}
			return nil
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if !root {
			if se.Name.Local != "nmaprun" {
				return errors.New("scanimport: nmap-xml: no nmaprun element")
			}
			root = true
			c.res.Tool = ToolInfo{Name: "nmap"}
			for _, a := range se.Attr {
				if a.Name.Local == "version" {
					c.res.Tool.Version = versionToken(a.Value)
				}
			}
			continue
		}
		if se.Name.Local != "host" {
			continue
		}
		var h nmapHost
		if err := dec.DecodeElement(&h, &se); err != nil {
			c.res.Truncated = true
			return nil
		}
		c.res.Records++
		if c.full() {
			return nil
		}
		c.nmapHost(h, fmt.Sprintf("host[%d]", idx))
		idx++
	}
}

// nmapWebScheme returns the URL scheme of an nmap web service, "" otherwise.
func nmapWebScheme(name, tunnel string) string {
	switch name {
	case "http", "http-alt", "http-proxy":
		if tunnel == "ssl" {
			return "https"
		}
		return "http"
	case "https", "https-alt":
		return "https"
	}
	return ""
}

func (c *collector) nmapHost(h nmapHost, rec string) {
	if h.Status.State != "up" {
		return
	}
	ip := ""
	for _, a := range h.Addrs {
		if ip == "" && (a.Type == "ipv4" || a.Type == "ipv6") && net.ParseIP(a.Addr) != nil {
			ip = Host(a.Addr)
		}
	}
	if ip == "" {
		c.res.Skipped++
		return
	}
	var open []Port
	var svcs []Asset
	var banners, names, oses []map[string]string
	for _, n := range h.Names {
		names = append(names, map[string]string{"name": n.Name, "type": n.Type})
	}
	for _, o := range h.OS {
		oses = append(oses, map[string]string{"name": o.Name, "accuracy": o.Accuracy})
	}
	for _, p := range h.Ports {
		port, err := strconv.Atoi(p.ID)
		if err != nil || port <= 0 || port > 65535 || p.State.State != "open" ||
			(p.Proto != "tcp" && p.Proto != "udp" && p.Proto != "sctp") {
			continue
		}
		name := Token(p.Svc.Name)
		if name == "" {
			name = "unknown"
		}
		open = append(open, Port{Port: port, Proto: p.Proto, Service: name})
		banners = append(banners, map[string]string{"port": fmt.Sprintf("%d/%s", port, p.Proto), "service": p.Svc.Name,
			"product": p.Svc.Product, "version": p.Svc.Version, "extrainfo": p.Svc.Extra, "tunnel": p.Svc.Tunnel,
			"cpe": strings.Join(p.Svc.CPE, " ")})
		prec := fmt.Sprintf("%s.port[%d/%s]", rec, port, p.Proto)
		if scheme := nmapWebScheme(name, p.Svc.Tunnel); scheme != "" && p.Proto == "tcp" {
			// Origin drops a default port, so http://ip:80 is the http://ip
			// another scanner reports.
			svcs = append(svcs, Asset{Kind: KindHTTPService, IP: ip,
				URL: Origin(scheme + "://" + net.JoinHostPort(ip, strconv.Itoa(port))), Record: prec})
		} else {
			svcs = append(svcs, Asset{Kind: KindService, IP: ip, Port: port, Proto: p.Proto, Service: name, Record: prec})
		}
	}
	c.asset(Asset{Kind: KindIP, IP: ip, OpenPorts: open, Record: rec,
		Detail: buildDetail(map[string]any{"ports": banners, "hostnames": names, "os": oses})})
	for _, n := range h.Names {
		// Only the name the operator scanned is bound to the IP; a PTR name is
		// third-party data and is ignored.
		if hn := Host(n.Name); n.Type == "user" && hn != "" && net.ParseIP(hn) == nil {
			c.asset(Asset{Kind: KindSubdomain, Domain: hn, IP: ip, Record: rec})
		}
	}
	for _, s := range svcs {
		c.asset(s)
	}
}
