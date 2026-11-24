package intercept

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Technique classes an engagement authorization artifact can allow. A tool
// call is put in every class whose signal it matches; a call that matches none
// is in no class and is not limited by the technique list (the scope, the
// window and the rest of the gate still apply).
//
// The classification reads what the call declares: the tool name and its
// command line (the command of a shell tool; command, cmd or args otherwise). Like target extraction it is a declared
// check, not a sandbox: a renamed binary or a script that runs a scanner is
// not seen. The list is data so it can grow without touching the gate.
const (
	TechniqueRecon             = "recon"
	TechniqueScanning          = "scanning"
	TechniqueFuzzing           = "fuzzing"
	TechniqueExploitation      = "exploitation"
	TechniqueCredentialAttack  = "credential_attack"
	TechniquePostExploitation  = "post_exploitation"
	TechniqueDenialOfService   = "denial_of_service"
	techniqueWordBoundaryLeft  = `(?i)(^|[^\w.-])(`
	techniqueWordBoundaryRight = `)([^\w.-]|$)`
)

type techniqueSignal struct {
	class string
	re    *regexp.Regexp
}

// toolWords builds a signal that matches any of the words as a whole word.
func toolWords(class string, words ...string) techniqueSignal {
	return techniqueSignal{class, regexp.MustCompile(techniqueWordBoundaryLeft + strings.Join(words, "|") + techniqueWordBoundaryRight)}
}

var techniqueSignals = []techniqueSignal{
	toolWords(TechniqueRecon, "dig", "nslookup", "whois", "subfinder", "amass", "assetfinder", "dnsx", "dnsrecon",
		"theharvester", "waybackurls", "gau", "shodan", "censys", "bbot"),
	toolWords(TechniqueScanning, "nmap", "masscan", "naabu", "rustscan", "zmap", "nuclei", "nikto", "httpx",
		"whatweb", "wpscan", "testssl", "testssl.sh", "sslscan", "sslyze", "katana", "zap-baseline.py", "enum4linux"),
	toolWords(TechniqueFuzzing, "ffuf", "gobuster", "dirsearch", "feroxbuster", "wfuzz", "dirb", "arjun", "kiterunner"),
	toolWords(TechniqueExploitation, "sqlmap", "dalfox", "commix", "xsstrike", "msfconsole", "msfvenom", "metasploit",
		"tplmap", "ysoserial", "ghauri", "nosqlmap"),
	toolWords(TechniqueCredentialAttack, "hydra", "medusa", "ncrack", "patator", "crackmapexec", "netexec", "nxc",
		"kerbrute", "hashcat", "sprayhound"),
	toolWords(TechniquePostExploitation, "mimikatz", "linpeas", "linpeas.sh", "winpeas", "chisel", "ligolo", "ligolo-ng",
		"secretsdump.py", "impacket-secretsdump", "psexec.py", "impacket-psexec", "evil-winrm", "bloodhound-python"),
	{TechniquePostExploitation, regexp.MustCompile(`/dev/tcp/|\b(nc|ncat)\b[^;&|]*\s-e(\s|$)`)},
	toolWords(TechniqueDenialOfService, "slowloris", "slowhttptest", "t50", "goldeneye", "siege"),
	{TechniqueDenialOfService, regexp.MustCompile(`(?i)\bhping3\b[^;&|]*--flood|\s--flood(\s|$)`)},
}

// TechniqueClasses lists every class an artifact may name, in a fixed order.
func TechniqueClasses() []string {
	return []string{TechniqueRecon, TechniqueScanning, TechniqueFuzzing, TechniqueExploitation,
		TechniqueCredentialAttack, TechniquePostExploitation, TechniqueDenialOfService}
}

// ClassifyTechniques returns the classes of one tool call, without duplicates,
// in the order of TechniqueClasses.
func ClassifyTechniques(toolName string, input []byte) []string {
	toolName, input = unwrapExtra(toolName, input)
	if noTargetTools[toolName] {
		return nil
	}
	// The name of an MCP or custom tool (mcp__scanner__nmap_scan) is read as words.
	text := strings.NewReplacer("__", " ", "_", " ").Replace(toolName)
	switch toolName {
	case "Bash", "shell_open", "Monitor":
		text += " " + declaredFields(input).Command
	case "shell_send":
		text += " " + declaredFields(input).Text
	case "WebFetch":
	default:
		// Other tools: the fields that carry a command line by convention. Free
		// text (a note, a name) is not read, so a word in it cannot misclassify.
		var in struct {
			Command string `json:"command"`
			Cmd     string `json:"cmd"`
			Args    any    `json:"args"`
		}
		if json.Unmarshal(input, &in) == nil {
			args, _ := json.Marshal(in.Args)
			text += " " + in.Command + " " + in.Cmd + " " + string(args)
		}
	}
	hit := map[string]bool{}
	for _, s := range techniqueSignals {
		if !hit[s.class] && s.re.MatchString(text) {
			hit[s.class] = true
		}
	}
	var out []string
	for _, c := range TechniqueClasses() {
		if hit[c] {
			out = append(out, c)
		}
	}
	return out
}
