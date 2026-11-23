package intercept

import "testing"

// A bare host name in the arguments of a known network tool is a target in a
// command: nmap host, curl host/x, ssh user@host, scp f host:/p.
func TestExtractBareHostOfNetworkTool(t *testing.T) {
	app := func(port int) Target { return Target{TargetDomain, "app.example.com", port} }
	for _, c := range []struct {
		cmd  string
		want []Target
	}{
		{"nmap app.example.com", []Target{app(0)}},
		{"nmap -sV -p 80,443 --script vuln -oN scan.nmap App.Example.com", []Target{app(0)}},
		{"ping -c1 app.example.com", []Target{app(0)}},
		{"curl app.example.com/login", []Target{app(0)}},
		{"curl -sk -H 'Host: other.example.com' -o page.html app.example.com/x", []Target{app(0)}},
		{"ssh root@app.example.com", []Target{app(0)}},
		{"ssh -i id.key -o StrictHostKeyChecking=no -J jump.example.net root@app.example.com",
			[]Target{{TargetDomain, "jump.example.net", 0}, app(0)}},
		{"scp loot.tgz admin@app.example.com:/tmp/", []Target{app(0)}},
		{"rsync -a d/ app.example.com:backup", []Target{app(0)}},
		{"nc -zv app.example.com 443", []Target{app(0)}},
		{"sudo timeout 30 nmap app.example.com", []Target{app(0)}},
		{"proxychains4 -q curl app.example.com", []Target{app(0)}},
		{"cd /tmp && nmap app.example.com | tee out.txt", []Target{app(0)}},
		{"sqlmap --url=app.example.com/item?id=1 --batch", []Target{app(0)}},
		{"impacket-secretsdump corp/admin@dc01.corp.example", []Target{{TargetDomain, "dc01.corp.example", 0}}},
		{"curl localhost/admin", []Target{{TargetDomain, "localhost", 0}}},
		// host:port read by the address rules and here: one target.
		{"curl app.example.com:8080/x", []Target{app(8080)}},
		// A name with a scheme is read by the URL rule only.
		{"curl https://app.example.com/", []Target{app(443)}},
	} {
		wantTargets(t, "Bash "+c.cmd, targetsOf("Bash", command(c.cmd)), c.want...)
		wantTargets(t, "shell_send "+c.cmd, targetsOf("shell_send", map[string]string{"text": c.cmd + "\n"}), c.want...)
	}
}

// What stays out: other programs, file names, flags' file values, words without
// a dot, DNS tools, and words the shell expands.
func TestExtractBareHostFalsePositives(t *testing.T) {
	for _, cmd := range []string{
		"cat app.example.com",                      // not a network tool
		"python3 exploit.py",                       // not a network tool
		"nmap -iL targets.txt -oA scan.out",        // file values and file names
		"ffuf -w common.txt -u $URL",               // a wordlist and a parameter
		"curl -d @payload.json -X POST $TARGET",    // data from a file
		"curl -u admin:secret $TARGET",             // credentials, no dot
		"hydra -l admin -P rockyou.lst ssh://$IP",  // file values
		"ssh -F ssh.config $HOST",                  // a config file
		"dig app.example.com; whois example.com",   // DNS tools ask a resolver
		"nmap ./hosts.list ~/x.y $T.example.com",   // paths and expansions
		"curl \"$BASE\"/api.example.com",           // the host is expanded
		"nc db 5432",                               // single label
		"mysql -u root -pS3cret.Pass --version",    // a flag's attached value
		"ping -c 3 1.2.3",                          // not a name
		"curl -o result.txt -K curl.conf $TARGET",  // file values
		"wget -i urls.lst",                         // a list of URLs in a file
		"nmap --script=http-title.nse 203.0.113.7", // a script, the address is read by the address rule
	} {
		got := targetsOf("Bash", command(cmd))
		for _, tg := range got {
			if tg.Kind == TargetDomain {
				t.Errorf("%s: read %+v as a host", cmd, tg)
			}
		}
	}
}

func TestBareHost(t *testing.T) {
	for _, c := range []struct {
		arg  string
		want Target
		ok   bool
	}{
		{"app.example.com", Target{TargetDomain, "app.example.com", 0}, true},
		{"root@app.example.com:2222", Target{TargetDomain, "app.example.com", 2222}, true},
		{"app.example.com:/srv", Target{TargetDomain, "app.example.com", 0}, true},
		{"@ns1.example.net", Target{TargetDomain, "ns1.example.net", 0}, true},
		{"report.pdf", Target{}, false},
		{"v1.2", Target{}, false},
		{"203.0.113.7", Target{}, false},
		{"2001:db8::1", Target{}, false},
		{"/etc/hosts", Target{}, false},
		{"x_y", Target{}, false},
		{"", Target{}, false},
	} {
		got, ok := bareHost(c.arg)
		if ok != c.ok || got != c.want {
			t.Errorf("bareHost(%q) = %+v, %v; want %+v, %v", c.arg, got, ok, c.want, c.ok)
		}
	}
}
