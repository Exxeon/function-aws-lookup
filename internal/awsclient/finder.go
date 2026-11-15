// Package awsclient finds AWS resources by tag with the Resource Groups
// Tagging API.
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
	rgt "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rgttypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/giepa/function-aws-lookup/internal/lookup"
)

// defaultRegion is used for STS when no other region is known.
const defaultRegion = "us-east-1"

// A Finder returns the ARNs of the resources that match a query.
type Finder interface {
	Find(ctx context.Context, region string, q lookup.Query) ([]string, error)
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

// TaggingFinder is a Finder backed by the Resource Groups Tagging API.
type TaggingFinder struct {
	cfg aws.Config

	mu      sync.Mutex
	clients map[string]*rgt.Client
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

	return &TaggingFinder{cfg: cfg, clients: map[string]*rgt.Client{}}, nil
}

// Find returns the ARNs of the resources in region that match q.
func (f *TaggingFinder) Find(ctx context.Context, region string, q lookup.Query) ([]string, error) {
	in := &rgt.GetResourcesInput{
		ResourceTypeFilters: []string{q.Type.Filter},
		TagFilters:          TagFilters(q.Tags),
	}

	var arns []string
	p := rgt.NewGetResourcesPaginator(f.client(region), in)
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

func (f *TaggingFinder) client(region string) *rgt.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if c, ok := f.clients[region]; ok {
		return c
	}
	c := rgt.NewFromConfig(f.cfg, func(o *rgt.Options) { o.Region = region })
	f.clients[region] = c
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
