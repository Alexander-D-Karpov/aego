package main

import (
	"errors"
	"fmt"
)

// verifySiteInput binds the production docs build to one release, its source
// subset, commit, and the two public origins. It is not release-gate evidence
// or proof that the entire Docker build context equals that commit.
func verifySiteInput(dir, root, expected, commit, appOrigin, docsOrigin string) (buildManifest, string, error) {
	var empty buildManifest
	if !shaPattern.MatchString(expected) || !commitPattern.MatchString(commit) {
		return empty, "", errors.New("site build requires a trusted manifest SHA-256 and a full source commit")
	}
	if !validOrigin(appOrigin, true) || !validOrigin(docsOrigin, true) || appOrigin == docsOrigin {
		return empty, "", errors.New("site build requires distinct exact HTTPS app and docs origins")
	}
	m, hash, err := verify(dir, expected)
	if err != nil {
		return empty, "", err
	}
	if !m.Release || m.Commit != commit || m.AppOrigin != appOrigin {
		return empty, "", errors.New("artifact release, commit, or application origin does not match the site build")
	}
	if len(m.DocsOrigins) != 2 || !((m.DocsOrigins[0] == docsOrigin && m.DocsOrigins[1] == appOrigin) ||
		(m.DocsOrigins[0] == appOrigin && m.DocsOrigins[1] == docsOrigin)) {
		return empty, "", errors.New("artifact parent origins do not match the site build")
	}
	source, err := sourceHash(root)
	if err != nil {
		return empty, "", fmt.Errorf("site source inventory: %w", err)
	}
	if source != m.SourceSHA256 {
		return empty, "", errors.New("site source subset differs from the artifact")
	}
	return m, hash, nil
}
