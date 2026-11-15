// Package lookup parses aws-lookup URIs and turns the ARNs of matching AWS
// resources into Crossplane external names.
package lookup

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Scheme of an aws-lookup URI.
const Scheme = "aws-lookup"

// Query is a parsed aws-lookup URI, for example
// aws-lookup://rds/instance?team=payments&env=prod#DbiResourceId.
type Query struct {
	// Type of the AWS resource to find.
	Type ResourceType

	// Tags the resource must have. A tag with several values matches any of
	// them. A tag with no values matches any value.
	Tags map[string][]string

	// Field is an optional Cloud Control property to use as the external
	// name, for example DbiResourceId or Endpoint/Address.
	Field string
}

// IsURI reports whether s looks like an aws-lookup URI.
func IsURI(s string) bool {
	return strings.HasPrefix(s, Scheme+"://")
}

// Parse an aws-lookup URI.
func Parse(s string) (Query, error) {
	u, err := url.Parse(s)
	if err != nil {
		return Query{}, fmt.Errorf("cannot parse %q: %w", s, err)
	}
	if u.Scheme != Scheme {
		return Query{}, fmt.Errorf("%q: scheme must be %s", s, Scheme)
	}

	service := u.Host
	kind := strings.Trim(u.Path, "/")
	if service == "" || kind == "" {
		return Query{}, fmt.Errorf("%q: must have the form %s://<service>/<type>?<tag>=<value>[#<field>]", s, Scheme)
	}

	t, ok := Lookup(service, kind)
	if !ok {
		return Query{}, fmt.Errorf("%q: unsupported resource type %s/%s, supported types are %s", s, service, kind, strings.Join(Supported(), ", "))
	}

	values, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return Query{}, fmt.Errorf("%q: cannot parse tags: %w", s, err)
	}
	if len(values) == 0 {
		return Query{}, fmt.Errorf("%q: must have at least one tag", s)
	}

	tags := make(map[string][]string, len(values))
	for k, vs := range values {
		if k == "" {
			return Query{}, fmt.Errorf("%q: tag keys must not be empty", s)
		}
		tags[k] = nil
		for _, v := range vs {
			if v != "" {
				tags[k] = append(tags[k], v)
			}
		}
	}

	if u.Fragment != "" {
		if _, err := fieldPath(u.Fragment); err != nil {
			return Query{}, fmt.Errorf("%q: %w", s, err)
		}
	}

	return Query{Type: t, Tags: tags, Field: u.Fragment}, nil
}

// ExternalNameField returns the Cloud Control property that holds the
// external name, or an empty string if no Cloud Control call is needed.
func (q Query) ExternalNameField() string {
	if q.Field != "" {
		return q.Field
	}
	return q.Type.DefaultField
}

// String returns the query as a canonical aws-lookup URI, with sorted tags.
func (q Query) String() string {
	keys := make([]string, 0, len(q.Tags))
	for k := range q.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	params := make([]string, 0, len(keys))
	for _, k := range keys {
		if len(q.Tags[k]) == 0 {
			params = append(params, url.QueryEscape(k)+"=")
			continue
		}
		for _, v := range q.Tags[k] {
			params = append(params, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}

	s := fmt.Sprintf("%s://%s/%s?%s", Scheme, q.Type.Service, q.Type.Name, strings.Join(params, "&"))
	if q.Field != "" {
		s += "#" + q.Field
	}
	return s
}
