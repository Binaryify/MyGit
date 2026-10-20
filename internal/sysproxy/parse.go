package sysproxy

import "strings"

// parseScutil reads scutil --proxy:
//
//	<dictionary> {
//	  ExceptionsList : <array> {
//	    0 : *.local
//	  }
//	  HTTPSEnable : 1
//	  HTTPSPort : 9004
//	  HTTPSProxy : proxy.example.com
//	}
func parseScutil(out string) config {
	v := map[string]string{}
	var c config
	inExceptions := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		key, val, ok := strings.Cut(line, " : ")
		switch {
		case line == "}":
			inExceptions = false
		case !ok:
		case inExceptions:
			c.bypass = append(c.bypass, val)
		case key == "ExceptionsList":
			inExceptions = true
		default:
			v[key] = val
		}
	}
	at := func(kind string) string {
		if v[kind+"Enable"] != "1" || v[kind+"Proxy"] == "" {
			return ""
		}
		if v[kind+"Port"] == "" {
			return v[kind+"Proxy"]
		}
		return v[kind+"Proxy"] + ":" + v[kind+"Port"]
	}
	c.http, c.https, c.socks = at("HTTP"), at("HTTPS"), at("SOCKS")
	return c
}

// parseWindows reads Internet Options' ProxyServer, host:port for every
// scheme or http=host:port;https=host:port;socks=host:port, and its
// ProxyOverride, the hosts without it separated by semicolons.
func parseWindows(server, override string) config {
	var c config
	server = strings.TrimSpace(server)
	if !strings.Contains(server, "=") {
		c.http, c.https = server, server
	} else {
		for _, part := range strings.Split(server, ";") {
			scheme, addr, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok {
				continue
			}
			switch strings.ToLower(scheme) {
			case "http":
				c.http = addr
			case "https":
				c.https = addr
			case "socks":
				c.socks = addr
			}
		}
	}
	if override != "" {
		c.bypass = strings.Split(override, ";")
	}
	return c
}
