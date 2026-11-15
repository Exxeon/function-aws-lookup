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

	// CloudFormationType is the type name for the Cloud Control API, for
	// example AWS::S3::Bucket.
	CloudFormationType string

	// CloudControlUsesARN means the Cloud Control primary identifier is the
	// full ARN, not the identifier.
	CloudControlUsesARN bool

	// DefaultField is the Cloud Control property to use as the external name
	// when the URI has no field. Leave it empty when the identifier or ARN is
	// the external name, so that no Cloud Control call is needed.
	DefaultField string
}

// A Resource found by a lookup.
type Resource struct {
	// ARN of the resource.
	ARN string

	// ID is the identifier in the ARN, for example vpc-0123 or my-bucket.
	ID string
}

// The external names below follow provider-upjet-aws. Only add a type after
// checking its external name configuration in that provider, and the primary
// identifier of its CloudFormation type.
//
//nolint:gochecknoglobals // A read-only lookup table.
var types = []ResourceType{
	{Service: "dynamodb", Name: "table", Filter: "dynamodb:table", Prefix: "table/", CloudFormationType: "AWS::DynamoDB::Table"},
	{Service: "ec2", Name: "instance", Filter: "ec2:instance", Prefix: "instance/", CloudFormationType: "AWS::EC2::Instance"},
	{Service: "ec2", Name: "internet-gateway", Filter: "ec2:internet-gateway", Prefix: "internet-gateway/", CloudFormationType: "AWS::EC2::InternetGateway"},
	{Service: "ec2", Name: "nat-gateway", Filter: "ec2:natgateway", Prefix: "natgateway/", CloudFormationType: "AWS::EC2::NatGateway"},
	{Service: "ec2", Name: "route-table", Filter: "ec2:route-table", Prefix: "route-table/", CloudFormationType: "AWS::EC2::RouteTable"},
	{Service: "ec2", Name: "security-group", Filter: "ec2:security-group", Prefix: "security-group/", CloudFormationType: "AWS::EC2::SecurityGroup"},
	{Service: "ec2", Name: "subnet", Filter: "ec2:subnet", Prefix: "subnet/", CloudFormationType: "AWS::EC2::Subnet"},
	{Service: "ec2", Name: "vpc", Filter: "ec2:vpc", Prefix: "vpc/", CloudFormationType: "AWS::EC2::VPC"},
	{Service: "ecr", Name: "repository", Filter: "ecr:repository", Prefix: "repository/", CloudFormationType: "AWS::ECR::Repository"},
	{Service: "efs", Name: "file-system", Filter: "elasticfilesystem:file-system", Prefix: "file-system/", CloudFormationType: "AWS::EFS::FileSystem"},
	{Service: "eks", Name: "cluster", Filter: "eks:cluster", Prefix: "cluster/", CloudFormationType: "AWS::EKS::Cluster"},
	{Service: "elasticache", Name: "replication-group", Filter: "elasticache:replicationgroup", Prefix: "replicationgroup:", CloudFormationType: "AWS::ElastiCache::ReplicationGroup"},
	{Service: "elb", Name: "load-balancer", Filter: "elasticloadbalancing:loadbalancer", Prefix: "loadbalancer/", UseARN: true, Match: isV2LoadBalancer, CloudFormationType: "AWS::ElasticLoadBalancingV2::LoadBalancer", CloudControlUsesARN: true},
	{Service: "elb", Name: "target-group", Filter: "elasticloadbalancing:targetgroup", Prefix: "targetgroup/", UseARN: true, CloudFormationType: "AWS::ElasticLoadBalancingV2::TargetGroup", CloudControlUsesARN: true},
	{Service: "kms", Name: "key", Filter: "kms:key", Prefix: "key/", CloudFormationType: "AWS::KMS::Key"},
	{Service: "lambda", Name: "function", Filter: "lambda:function", Prefix: "function:", Match: noSeparators, CloudFormationType: "AWS::Lambda::Function"},
	{Service: "logs", Name: "log-group", Filter: "logs:log-group", Prefix: "log-group:", CloudFormationType: "AWS::Logs::LogGroup"},
	{Service: "rds", Name: "cluster", Filter: "rds:cluster", Prefix: "cluster:", CloudFormationType: "AWS::RDS::DBCluster"},
	// Since Terraform AWS provider v5 the ID of aws_db_instance, and so the
	// external name, is the DBI resource ID, which is not in the ARN.
	{Service: "rds", Name: "instance", Filter: "rds:db", Prefix: "db:", CloudFormationType: "AWS::RDS::DBInstance", DefaultField: "DbiResourceId"},
	{Service: "s3", Name: "bucket", Filter: "s3", Match: noSeparators, CloudFormationType: "AWS::S3::Bucket"},
	{Service: "secretsmanager", Name: "secret", Filter: "secretsmanager:secret", Prefix: "secret:", UseARN: true, CloudFormationType: "AWS::SecretsManager::Secret", CloudControlUsesARN: true},
	{Service: "sns", Name: "topic", Filter: "sns", Match: noSeparators, CloudFormationType: "AWS::SNS::Topic", CloudControlUsesARN: true},
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

// Resource parses an ARN returned by the Tagging API. It returns false if the
// ARN is not of this type.
func (t ResourceType) Resource(s string) (Resource, bool, error) {
	a, err := arn.Parse(s)
	if err != nil {
		return Resource{}, false, fmt.Errorf("cannot parse ARN %q: %w", s, err)
	}

	service, _, _ := strings.Cut(t.Filter, ":")
	if a.Service != service || !strings.HasPrefix(a.Resource, t.Prefix) {
		return Resource{}, false, nil
	}

	id := strings.TrimPrefix(a.Resource, t.Prefix)
	if t.Service == "logs" {
		id = strings.TrimSuffix(id, ":*")
	}
	if id == "" || (t.Match != nil && !t.Match(id)) {
		return Resource{}, false, nil
	}

	return Resource{ARN: s, ID: id}, true, nil
}

// ExternalName returns the external name of r, for types that don't need a
// Cloud Control call.
func (t ResourceType) ExternalName(r Resource) string {
	if t.UseARN {
		return r.ARN
	}
	return r.ID
}

// CloudControlIdentifier returns the Cloud Control primary identifier of r.
func (t ResourceType) CloudControlIdentifier(r Resource) string {
	if t.CloudControlUsesARN {
		return r.ARN
	}
	return r.ID
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
