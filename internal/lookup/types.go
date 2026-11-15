package lookup

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
)

// ResourceType is an AWS resource type that the Function can look up.
type ResourceType struct {
	// Service and Name identify the type in an aws-lookup URI, for example
	// aws-lookup://<service>/<name>.
	Service string
	Name    string

	// Filter is the resource type filter for the Resource Groups Tagging API,
	// in the form service[:resourceType].
	Filter string

	// Prefix is the part of the ARN resource that comes before the
	// identifier, for example "vpc/" in arn:aws:ec2:...:vpc/vpc-0123.
	Prefix string

	// UseARN means the external name is the full ARN, not the identifier.
	UseARN bool

	// Match optionally rejects ARNs that pass the filter but belong to a
	// different kind of resource. It receives the identifier.
	Match func(id string) bool
}

// The external names below follow provider-upjet-aws. Only add a type after
// checking its external name configuration in that provider.
var types = []ResourceType{ //nolint:gochecknoglobals // A read-only lookup table.
	{Service: "dynamodb", Name: "table", Filter: "dynamodb:table", Prefix: "table/"},
	{Service: "ec2", Name: "instance", Filter: "ec2:instance", Prefix: "instance/"},
	{Service: "ec2", Name: "internet-gateway", Filter: "ec2:internet-gateway", Prefix: "internet-gateway/"},
	{Service: "ec2", Name: "nat-gateway", Filter: "ec2:natgateway", Prefix: "natgateway/"},
	{Service: "ec2", Name: "route-table", Filter: "ec2:route-table", Prefix: "route-table/"},
	{Service: "ec2", Name: "security-group", Filter: "ec2:security-group", Prefix: "security-group/"},
	{Service: "ec2", Name: "subnet", Filter: "ec2:subnet", Prefix: "subnet/"},
	{Service: "ec2", Name: "vpc", Filter: "ec2:vpc", Prefix: "vpc/"},
	{Service: "ecr", Name: "repository", Filter: "ecr:repository", Prefix: "repository/"},
	{Service: "efs", Name: "file-system", Filter: "elasticfilesystem:file-system", Prefix: "file-system/"},
	{Service: "eks", Name: "cluster", Filter: "eks:cluster", Prefix: "cluster/"},
	{Service: "elasticache", Name: "replication-group", Filter: "elasticache:replicationgroup", Prefix: "replicationgroup:"},
	{Service: "elb", Name: "load-balancer", Filter: "elasticloadbalancing:loadbalancer", Prefix: "loadbalancer/", UseARN: true, Match: isV2LoadBalancer},
	{Service: "elb", Name: "target-group", Filter: "elasticloadbalancing:targetgroup", Prefix: "targetgroup/", UseARN: true},
	{Service: "kms", Name: "key", Filter: "kms:key", Prefix: "key/"},
	{Service: "lambda", Name: "function", Filter: "lambda:function", Prefix: "function:", Match: noSeparators},
	{Service: "logs", Name: "log-group", Filter: "logs:log-group", Prefix: "log-group:"},
	{Service: "rds", Name: "cluster", Filter: "rds:cluster", Prefix: "cluster:"},
	{Service: "s3", Name: "bucket", Filter: "s3", Match: noSeparators},
	{Service: "secretsmanager", Name: "secret", Filter: "secretsmanager:secret", Prefix: "secret:", UseARN: true},
	{Service: "sns", Name: "topic", Filter: "sns", Match: noSeparators},
}

// Lookup returns the resource type for service and name.
func Lookup(service, name string) (ResourceType, bool) {
	for _, t := range types {
		if t.Service == service && t.Name == name {
			return t, true
		}
	}
	return ResourceType{}, false
}

// Supported returns the supported types in the form service/name.
func Supported() []string {
	s := make([]string, 0, len(types))
	for _, t := range types {
		s = append(s, t.Service+"/"+t.Name)
	}
	sort.Strings(s)
	return s
}

// ExternalName returns the Crossplane external name for an ARN. It returns
// false if the ARN is not of this type.
func (t ResourceType) ExternalName(s string) (string, bool, error) {
	a, err := arn.Parse(s)
	if err != nil {
		return "", false, fmt.Errorf("cannot parse ARN %q: %w", s, err)
	}

	service, _, _ := strings.Cut(t.Filter, ":")
	if a.Service != service || !strings.HasPrefix(a.Resource, t.Prefix) {
		return "", false, nil
	}

	id := strings.TrimPrefix(a.Resource, t.Prefix)
	if t.Service == "logs" {
		id = strings.TrimSuffix(id, ":*")
	}
	if id == "" || (t.Match != nil && !t.Match(id)) {
		return "", false, nil
	}

	if t.UseARN {
		return s, true, nil
	}
	return id, true, nil
}

// noSeparators rejects sub-resources such as S3 access points or Lambda
// function versions.
func noSeparators(id string) bool {
	return !strings.ContainsAny(id, "/:")
}

// isV2LoadBalancer accepts application, network and gateway load balancers
// (app/<name>/<id>) and rejects classic load balancers (<name>).
func isV2LoadBalancer(id string) bool {
	return strings.Count(id, "/") == 2
}
