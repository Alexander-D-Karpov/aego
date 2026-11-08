package remoteexample

import (
	"fmt"
	"net/url"
)

type Link struct {
	ManifestURL string
	Mode        string
}

// ParseLink only produces a request. It neither grants origin trust nor starts
// a project. A protocol handler must show the source and ask for confirmation.
func ParseLink(raw string) (Link, error) {
	var l Link
	if len(raw) > 8192 {
		return l, fmt.Errorf("deep link too long")
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "aego" || u.Host != "example" || u.User != nil || u.Path != "" || u.Fragment != "" || u.Opaque != "" {
		return l, fmt.Errorf("invalid example deep link")
	}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return l, e
	}
	for k, v := range q {
		if (k != "manifest" && k != "mode") || len(v) != 1 {
			return l, fmt.Errorf("unsupported or repeated deep-link parameter")
		}
	}
	l.ManifestURL = q.Get("manifest")
	l.Mode = q.Get("mode")
	if l.Mode == "" {
		l.Mode = "project"
	}
	if l.Mode != "project" && l.Mode != "scene" {
		return l, fmt.Errorf("unsupported example operation")
	}
	if _, e := httpsURL(l.ManifestURL); e != nil {
		return l, e
	}
	return l, nil
}
func MakeLink(raw, mode string) (string, error) {
	if _, e := httpsURL(raw); e != nil {
		return "", e
	}
	if mode != "project" && mode != "scene" {
		return "", fmt.Errorf("unsupported example operation")
	}
	return "aego://example?" + url.Values{"manifest": {raw}, "mode": {mode}}.Encode(), nil
}
