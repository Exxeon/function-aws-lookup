// Package awsclient finds AWS resources by tag with the Resource Groups
// Tagging API, and reads their properties with the Cloud Control API.
package awsclient

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	rgt "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rgttypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/giepa/function-aws-lookup/internal/lookup"
)

// defaultRegion is used for STS when no other region is known.
const defaultRegion = "us-east-1"

// A Finder finds AWS resources and reads their properties.
type Finder interface {
	// Find returns the ARNs of the resources that match a query.
	Find(ctx context.Context, region string, q lookup.Query) ([]string, error)

	// Properties returns the Cloud Control properties of a resource as JSON.
	Properties(ctx context.Context, region, typeName, identifier string) (string, error)
}

// Options configure a Finder.
type Options struct {
	// Credentials from the pipeline step. If nil, the default AWS credential
	// chain is used.
	Credentials map[string][]byte

	// AssumeRoleARN is an optional IAM role to assume.
	AssumeRoleARN string

	// Region for STS calls.
	Region string
}

// AWSFinder is a Finder backed by the Resource Groups Tagging API and the
// Cloud Control API.
type AWSFinder struct {
	cfg aws.Config

	mu            sync.Mutex
	tagging       map[string]*rgt.Client
	cloudControls map[string]*cloudcontrol.Client
}

// NewFinder returns a Finder that uses the supplied options.
func NewFinder(ctx context.Context, o Options) (Finder, error) {
	var opts []func(*config.LoadOptions) error
	if o.Credentials != nil {
		c, err := StaticCredentials(o.Credentials)
		if err != nil {
			return nil, err
		}
		opts = append(opts, config.WithCredentialsProvider(credentials.StaticCredentialsProvider{Value: c}))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("cannot load AWS configuration: %w", err)
	}
	if cfg.Region == "" {
		cfg.Region = o.Region
	}
	if cfg.Region == "" {
		cfg.Region = defaultRegion
	}

	if o.AssumeRoleARN != "" {
		cfg.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), o.AssumeRoleARN))
	}

	return &AWSFinder{cfg: cfg, tagging: map[string]*rgt.Client{}, cloudControls: map[string]*cloudcontrol.Client{}}, nil
}

// Find returns the ARNs of the resources in region that match q.
func (f *AWSFinder) Find(ctx context.Context, region string, q lookup.Query) ([]string, error) {
	in := &rgt.GetResourcesInput{
		ResourceTypeFilters: []string{q.Type.Filter},
		TagFilters:          TagFilters(q.Tags),
	}

	var arns []string
	p := rgt.NewGetResourcesPaginator(f.taggingClient(region), in)
	for p.HasMorePages() {
		out, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("cannot get resources from the Resource Groups Tagging API in %s: %w", region, err)
		}
		for _, m := range out.ResourceTagMappingList {
			arns = append(arns, aws.ToString(m.ResourceARN))
		}
	}
	return arns, nil
}

// Properties returns the Cloud Control properties of a resource as JSON.
func (f *AWSFinder) Properties(ctx context.Context, region, typeName, identifier string) (string, error) {
	out, err := f.cloudControlClient(region).GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   aws.String(typeName),
		Identifier: aws.String(identifier),
	})
	if err != nil {
		return "", fmt.Errorf("cannot get %s %s from the Cloud Control API in %s: %w", typeName, identifier, region, err)
	}
	if out.ResourceDescription == nil || out.ResourceDescription.Properties == nil {
		return "", fmt.Errorf("the Cloud Control API returned no properties for %s %s", typeName, identifier)
	}
	return aws.ToString(out.ResourceDescription.Properties), nil
}

func (f *AWSFinder) taggingClient(region string) *rgt.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if c, ok := f.tagging[region]; ok {
		return c
	}
	c := rgt.NewFromConfig(f.cfg, func(o *rgt.Options) { o.Region = region })
	f.tagging[region] = c
	return c
}

func (f *AWSFinder) cloudControlClient(region string) *cloudcontrol.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if c, ok := f.cloudControls[region]; ok {
		return c
	}
	c := cloudcontrol.NewFromConfig(f.cfg, func(o *cloudcontrol.Options) { o.Region = region })
	f.cloudControls[region] = c
	return c
}

// TagFilters converts tags to Tagging API filters, sorted by key.
func TagFilters(tags map[string][]string) []rgttypes.TagFilter {
	filters := make([]rgttypes.TagFilter, 0, len(tags))
	for k, vs := range tags {
		filters = append(filters, rgttypes.TagFilter{Key: aws.String(k), Values: vs})
	}
	sort.Slice(filters, func(i, j int) bool { return aws.ToString(filters[i].Key) < aws.ToString(filters[j].Key) })
	return filters
}
