package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	storedConfName = "wireguard.conf"
	maxConfBytes   = 64 * 1024
)

func dataDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "DiscordGoLiveBypass"), nil
}

func storedConfPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, storedConfName), nil
}

// Basta para separar um arquivo do WireGuard de outro .conf qualquer que esteja nos
// Downloads. Quem valida de verdade é o parser, na hora de subir o túnel.
func looksLikeWireGuard(text string) bool {
	section := ""
	hasKey, hasPeer := false, false

	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if section == "peer" {
				hasPeer = true
			}
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if ok && section == "interface" && strings.EqualFold(strings.TrimSpace(key), "PrivateKey") {
			hasKey = true
		}
	}
	return hasKey && hasPeer
}

type confCandidate struct {
	Path     string
	Modified time.Time
}

// Do mais recente para o mais antigo: quem acabou de baixar o arquivo do Proton quer
// esse, e não um de meses atrás que ficou esquecido na pasta.
func findConfCandidates(dirs []string) []confCandidate {
	var found []confCandidate
	seen := map[string]bool{}

	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".conf") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if seen[strings.ToLower(path)] {
				continue
			}
			seen[strings.ToLower(path)] = true

			info, err := entry.Info()
			if err != nil || info.Size() > maxConfBytes {
				continue
			}
			raw, err := os.ReadFile(path)
			if err != nil || !looksLikeWireGuard(string(raw)) {
				continue
			}
			found = append(found, confCandidate{path, info.ModTime()})
		}
	}

	sort.SliceStable(found, func(i, j int) bool { return found[i].Modified.After(found[j].Modified) })
	return found
}

func sameFile(a, b string) (bool, error) {
	ia, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	ib, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(ia, ib), nil
}

// O Proton escreve o nome do servidor como comentário logo depois do [Peer], como
// "# US-FREE#57". É o que a pessoa reconhece, bem mais que um IP.
var peerLabelRe = regexp.MustCompile(`(?mi)^\s*\[Peer\]\s*\r?\n(?:\s*\r?\n)*\s*#\s*([^\r\n]{1,64}?)\s*$`)

func serverLabel(text string) string {
	if match := peerLabelRe.FindStringSubmatch(text); match != nil {
		return match[1]
	}
	return ""
}
