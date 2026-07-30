package filter

import (
	"strings"
	"unicode"
)

func IsJunkWord(w string) bool {
	junk := map[string]bool{
		"the": true, "and": true, "for": true, "new": true, "all": true,
		"our": true, "its": true, "has": true, "are": true, "was": true,
		"can": true, "not": true, "you": true, "your": true, "from": true,
		"that": true, "this": true, "with": true, "have": true, "been": true,
		"will": true, "more": true, "page": true, "home": true, "site": true,
		"need": true, "run": true, "app": true, "api": true, "use": true,
		"get": true, "one": true, "two": true, "see": true, "now": true,
		"com": true, "org": true, "net": true, "www": true,
		"enable": true, "javascript": true, "cookie": true, "function": true,
		"brand": true, "center": true, "rights": true, "reserved": true,
		"privacy": true, "policy": true, "terms": true, "contact": true,
		"about": true, "search": true, "menu": true, "close": true,
		"open": true, "login": true, "register": true, "sign": true,
		"subscribe": true, "newsletter": true, "follow": true, "share": true,
		"like": true, "comment": true, "download": true, "upload": true,
		"click": true, "here": true, "link": true, "skip": true,
		"content": true, "main": true, "navigation": true, "footer": true,
		"header": true, "sidebar": true, "related": true, "previous": true,
		"next": true, "back": true, "top": true, "read": true,
		"view": true, "web": true, "website": true, "online": true,
		"internet": true, "https": true, "http": true, "html": true,
		"css": true, "internal": true,
		"nginx": true, "apache": true, "cloudflare": true, "plesk": true,
		"plesklin": true, "cpanel": true, "wordpress": true, "jquery": true,
		"bootstrap": true, "react": true, "vue": true, "angular": true,
		"node": true, "express": true, "django": true, "laravel": true,
		"php": true, "mysql": true, "postgres": true, "redis": true,
		"docker": true, "kubernetes": true, "aws": true, "azure": true,
		"google": true, "analytics": true, "tag": true, "manager": true,
		"tech": true, "stack": true, "server": true, "hosting": true,
		"host": true, "cdn": true, "dns": true, "ssl": true,
	}
	return junk[strings.ToLower(strings.TrimSpace(w))]
}

func IsJunkCandidate(w string) bool {
	if strings.ContainsAny(w, " \t\n\r") {
		return true
	}
	if len(w) < 4 {
		return true
	}
	if IsAllDigits(w) {
		return true
	}
	hasLetter := false
	for _, r := range w {
		if unicode.IsLetter(r) {
			hasLetter = true
			break
		}
	}
	return !hasLetter
}

func IsAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func Deduplicate(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s == "" {
			continue
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
