package adapters

import (
	"fmt"
	"net/url"
	"strings"
)

func ApplyMongoDefaultDatabase(rawURI, defaultDatabase string) (string, error) {
	if defaultDatabase == "" {
		return rawURI, nil
	}

	scheme, rest, ok := strings.Cut(rawURI, "://")
	if !ok {
		return "", fmt.Errorf("parse MongoDB URI: missing scheme")
	}
	if scheme != "mongodb" && scheme != "mongodb+srv" {
		return "", fmt.Errorf("unsupported MongoDB URI scheme %q", scheme)
	}

	beforeQuery, query, hasQuery := strings.Cut(rest, "?")
	base := beforeQuery
	if beforePath, path, hasPath := strings.Cut(beforeQuery, "/"); hasPath {
		if path != "" {
			return rawURI, nil
		}
		base = beforePath
	}
	if base == "" {
		return "", fmt.Errorf("parse MongoDB URI: missing host")
	}

	activated := scheme + "://" + base + "/" + url.PathEscape(defaultDatabase)
	if hasQuery {
		activated += "?" + query
	}
	return activated, nil
}
