package lookup

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
)

// A Resource found by a lookup.
type Resource struct {
	// ARN of the resource.
	ARN string

	// ID is the part of the ARN after the resource type, for example
	// vpc-0123 in arn:aws:ec2:eu-west-1:123456789012:vpc/vpc-0123, or
	// my-bucket in arn:aws:s3:::my-bucket. It is the default external name.
	ID string
}

// Resource parses an ARN returned by the Tagging API. It returns false if the
// ARN is not of the queried service and type.
func (q Query) Resource(s string) (Resource, bool, error) {
	a, err := arn.Parse(s)
	if err != nil {
		return Resource{}, false, fmt.Errorf("cannot parse ARN %q: %w", s, err)
	}
	if a.Service != q.Service {
		return Resource{}, false, nil
	}

	// Some services, such as API Gateway, start the resource with a slash:
	// arn:aws:apigateway:eu-west-1::/restapis/a1b2c3.
	res := strings.TrimPrefix(a.Resource, "/")
	i := strings.IndexAny(res, ":/")

	if q.Type == "" {
		// Without a type, only resources whose ARN has no type match, so that
		// aws-lookup://s3 finds buckets but not access points.
		if i >= 0 {
			return Resource{}, false, nil
		}
		return Resource{ARN: s, ID: res}, true, nil
	}

	if i < 0 || res[:i] != q.Type {
		return Resource{}, false, nil
	}
	id := strings.TrimSuffix(res[i+1:], ":*")
	if id == "" {
		return Resource{}, false, nil
	}
	return Resource{ARN: s, ID: id}, true, nil
}
