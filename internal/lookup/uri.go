// Package lookup parses aws-lookup URIs and turns the ARNs of matching AWS
// resources into Crossplane external names.
package lookup

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Scheme of an aws-lookup URI.
const Scheme = "aws-lookup"

var (
	validService = regexp.MustCompile(`^[a-z0-9-]+$`)
	validType    = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// Query is a parsed aws-lookup URI, for example
// aws-lookup://rds/db?team=payments&env=prod#DbiResourceId.
type Query struct {
	// Service is the service in the ARN, for example rds.
	Service string

	// Type is the resource type in the ARN, for example db. It is empty for
	// resources whose ARN has no resource type, such as S3 buckets
	// (arn:aws:s3:::my-bucket) and SNS topics.
	Type string

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
	kind := strings.TrimPrefix(u.Path, "/")
	if !validService.MatchString(service) || (kind != "" && !validType.MatchString(kind)) {
		return Query{}, fmt.Errorf("%q: must have the form %s://<service>[/<type>]?<tag>=<value>[#<field>], with the service and type from the ARN", s, Scheme)
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

	return Query{Service: service, Type: kind, Tags: tags, Field: u.Fragment}, nil
}

// Filter returns the resource type filter for the Resource Groups Tagging
// API, in the form service[:type].
func (q Query) Filter() string {
	if q.Type == "" {
		return q.Service
	}
	return q.Service + ":" + q.Type
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

	path := ""
	if q.Type != "" {
		path = "/" + q.Type
	}
	s := fmt.Sprintf("%s://%s%s?%s", Scheme, q.Service, path, strings.Join(params, "&"))
	if q.Field != "" {
		s += "#" + q.Field
	}
	return s
}
