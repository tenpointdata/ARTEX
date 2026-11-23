package intercept

import (
	"path"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// The command-string rules (scan) do not read a bare host name: a file name
// such as x.py is a valid host name, so a bare name anywhere in a command
// would ask on every script run. Yet nmap app.example.com, curl
// app.example.com/x and ssh root@app.example.com name their target that way.
// scanCommand closes that gap for the programs whose arguments are network
// targets: a bare name is read only in the arguments of a known network tool,
// only when it has a dot and an alphabetic last label that is not a common
// file extension (or is localhost), and never from a flag's value that names
// a file, a header or data. user@host, host:port, host:path and host/path are
// read as the host (and the port).

// scanCommand reads a shell command: the command-string rules, then the bare
// host names in the arguments of known network tools.
func (l *targetList) scanCommand(s string) {
	l.scan(s)
	for _, t := range bareHostTargets(s) {
		l.add(t)
	}
}

// networkTools are the programs whose positional arguments name a network
// target. DNS tools (dig, host, nslookup, whois) are not listed: they ask a
// resolver or a registry, not the name they are given.
var networkTools = toolSet(
	"curl", "wget", "http", "https", "xh", "httpie", "wscat", "websocat",
	"nmap", "masscan", "rustscan", "naabu", "zmap", "nbtscan", "ike-scan",
	"ssh", "scp", "sftp", "rsync", "ssh-keyscan", "telnet", "ftp", "lftp", "tftp",
	"nc", "ncat", "netcat", "ping", "ping6", "fping", "hping3", "traceroute", "traceroute6", "tracepath", "mtr",
	"sqlmap", "nikto", "ffuf", "gobuster", "feroxbuster", "wfuzz", "dirb", "dirsearch", "nuclei", "httpx",
	"whatweb", "wpscan", "joomscan", "hydra", "medusa", "ncrack", "patator",
	"sslscan", "sslyze", "testssl", "testssl.sh", "tlsx",
	"mysql", "psql", "redis-cli", "mongo", "mongosh",
	"smbclient", "smbmap", "crackmapexec", "cme", "nxc", "netexec", "enum4linux", "enum4linux-ng",
	"rpcclient", "ldapsearch", "snmpwalk", "snmpget", "onesixtyone", "showmount", "evil-winrm", "xfreerdp", "rdesktop",
)

// commandWrappers run their arguments as a command: the program after them is
// the one whose arguments are read.
var commandWrappers = toolSet("sudo", "doas", "env", "timeout", "nice", "nohup", "stdbuf", "time", "command", "exec",
	"proxychains", "proxychains4", "torsocks", "unbuffer")

// fileValueFlags take a value that is a file, a header, data or a pattern,
// never the target: the word after them is not read.
var fileValueFlags = toolSet(
	"-o", "-O", "-oN", "-oX", "-oG", "-oA", "-oS", "-iL", "-w", "-W", "--wordlist", "--output", "--output-file",
	"-d", "--data", "--data-binary", "--data-raw", "--data-urlencode", "-F", "--form", "-T", "--upload-file",
	"-H", "--header", "-A", "--user-agent", "-e", "--referer", "-b", "--cookie", "-c", "--cookie-jar",
	"-K", "--config", "-E", "--cert", "--key", "--cacert", "--capath", "-i", "-l", "-L", "-P", "-C", "-r",
	"--script", "--script-args", "--resolve", "--connect-to", "--proxy-user", "-U",
)

// fileExtensions are last labels that name a file far more often than a
// top-level domain, in the arguments of a network tool.
var fileExtensions = toolSet(
	"txt", "json", "xml", "html", "htm", "csv", "log", "nmap", "gnmap", "nse", "conf", "cfg", "ini", "toml", "yaml", "yml",
	"pem", "key", "crt", "cer", "der", "pfx", "p12", "ovpn", "lst", "js", "php", "go", "rb", "py", "sh", "md",
	"out", "gz", "tgz", "zip", "tar", "bz2", "xz", "sql", "db", "sqlite", "bak", "pcap", "pcapng", "har",
	"jar", "war", "exe", "dll", "bin", "iso", "png", "jpg", "jpeg", "gif", "svg", "pdf", "req", "raw", "tmp",
	"hash", "lock", "env", "ps1", "bat",
)

var reDuration = regexp.MustCompile(`^\d+(\.\d+)?[smhd]?$`)

// bareHostTargets returns the bare host names the network tools of a command
// name. A command that does not parse (a fragment sent to an interactive
// shell) is split on blanks and shell operators instead.
func bareHostTargets(cmd string) []Target {
	var out []Target
	for _, argv := range commandArgvs(cmd) {
		out = append(out, toolArgTargets(argv)...)
	}
	return out
}

// commandArgvs returns the argv of every simple command in cmd. A word that
// holds an expansion (a parameter, a substitution) is kept as "", which no
// rule reads.
func commandArgvs(cmd string) [][]string {
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(cmd), "")
	if err != nil {
		var out [][]string
		for _, part := range strings.FieldsFunc(cmd, func(r rune) bool { return strings.ContainsRune(shellMeta+"\n", r) }) {
			if words := strings.Fields(part); len(words) > 0 {
				out = append(out, words)
			}
		}
		return out
	}
	var out [][]string
	syntax.Walk(f, func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		argv := make([]string, len(call.Args))
		for k, w := range call.Args {
			// A glob or an escape in a word keeps its text (curl host/x?id=1);
			// bareHost refuses a host part that holds one.
			if word, ok := argvWordOf(w); ok {
				argv[k] = word.Value
			}
		}
		out = append(out, argv)
		return true
	})
	return out
}

// toolArgTargets reads the bare hosts of one argv, when its program (after any
// wrapper) is a network tool.
func toolArgTargets(argv []string) []Target {
	for len(argv) > 0 && commandWrappers[path.Base(argv[0])] {
		argv = argv[1:]
		// The wrapper's own options, assignments and a timeout's duration.
		for len(argv) > 0 && (strings.HasPrefix(argv[0], "-") || strings.Contains(argv[0], "=") || reDuration.MatchString(argv[0])) {
			argv = argv[1:]
		}
	}
	if len(argv) == 0 {
		return nil
	}
	prog := path.Base(argv[0])
	if !networkTools[prog] && !strings.HasPrefix(prog, "impacket-") {
		return nil
	}
	var out []Target
	for k := 1; k < len(argv); k++ {
		arg := argv[k]
		if strings.HasPrefix(arg, "-") {
			if fileValueFlags[arg] {
				k++
				continue
			}
			// --url=host, --host=host: the value of a long option.
			if name, value, ok := strings.Cut(arg, "="); ok && strings.HasPrefix(name, "--") && !fileValueFlags[name] {
				arg = value
			} else {
				continue
			}
		}
		if t, ok := bareHost(arg); ok {
			out = append(out, t)
		}
	}
	return out
}

// bareHost reads one argument as [user@]host[:port][/path] or host:path. It
// reads only a plausible host name: see the comment at the top of the file.
func bareHost(arg string) (Target, bool) {
	arg = strings.TrimPrefix(arg, "@")
	if arg == "" || strings.Contains(arg, "://") || strings.ContainsAny(arg[:1], "./~$+") {
		return Target{}, false
	}
	arg = arg[strings.LastIndexByte(arg, '@')+1:]
	if i := strings.IndexByte(arg, '/'); i >= 0 {
		arg = arg[:i]
	}
	host, port := arg, 0
	if h, p, ok := strings.Cut(arg, ":"); ok {
		if strings.Contains(p, ":") {
			return Target{}, false // an IPv6 literal: the address rules read it
		}
		// host:port, else host:path (scp, rsync): the host alone.
		host, port = h, validPort(p)
	}
	kind, h, ok := normalizeHost(host, false)
	if !ok || kind != TargetDomain || !validHostname(h) || !plausibleHostName(h) {
		return Target{}, false
	}
	return Target{Kind: TargetDomain, Host: h, Port: port}, true
}

// plausibleHostName reports whether a valid host name reads as a host rather
// than a file or a word: localhost, or a dotted name whose last label is
// alphabetic, at least two letters and not a common file extension.
func plausibleHostName(h string) bool {
	if h == "localhost" {
		return true
	}
	i := strings.LastIndexByte(h, '.')
	if i <= 0 {
		return false
	}
	tld := h[i+1:]
	if len(tld) < 2 || fileExtensions[tld] {
		return false
	}
	for _, c := range []byte(tld) {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}
