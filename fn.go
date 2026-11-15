package main

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/crossplane/function-sdk-go/errors"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/Exxeon/function-aws-lookup/input/v1beta1"
	"github.com/Exxeon/function-aws-lookup/internal/awsclient"
	"github.com/Exxeon/function-aws-lookup/internal/lookup"
)

// AnnotationExternalName is the Crossplane external name annotation.
const AnnotationExternalName = "crossplane.io/external-name"

const (
	defaultCredentialsName = "aws-creds"
	maxListedMatches       = 5
)

// A FinderFactory creates a Finder.
type FinderFactory func(ctx context.Context, o awsclient.Options) (awsclient.Finder, error)

// Function resolves aws-lookup URIs in external name annotations.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer

	log       logging.Logger
	newFinder FinderFactory
}

// RunFunction runs the Function.
func (f *Function) RunFunction(ctx context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	log := f.log.WithValues("tag", req.GetMeta().GetTag())
	rsp := response.To(req, response.DefaultTTL)

	in := &v1beta1.Input{}
	if req.GetInput() != nil {
		if err := request.GetInput(req, in); err != nil {
			response.Fatal(rsp, errors.Wrapf(err, "cannot get Function input from %T", req))
			return rsp, nil
		}
	}
	if in.CredentialsName == "" {
		in.CredentialsName = defaultCredentialsName
	}
	if in.OnNotFound == "" {
		in.OnNotFound = v1beta1.NotFoundPolicyFail
	}

	desired, err := request.GetDesiredComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get desired composed resources"))
		return rsp, nil
	}
	observed, err := request.GetObservedComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get observed composed resources"))
		return rsp, nil
	}

	r := &resolver{newFinder: f.newFinder, req: req, in: in, matches: map[string][]lookup.Resource{}, properties: map[string]string{}}

	for _, name := range slices.Sorted(maps.Keys(desired)) {
		dr := desired[name]
		uri := dr.Resource.GetAnnotations()[AnnotationExternalName]
		if !lookup.IsURI(uri) {
			continue
		}
		log := log.WithValues("resource", name, "uri", uri)

		// Once the resource exists, keep the external name it already has.
		// This also means AWS is only called when a resource is first adopted.
		if ext := observedExternalName(observed, name); ext != "" {
			log.Debug("Keeping observed external name", "externalName", ext)
			setExternalName(dr, ext)
			continue
		}

		q, err := lookup.Parse(uri)
		if err != nil {
			response.Fatal(rsp, errors.Wrapf(err, "composed resource %q", name))
			return rsp, nil
		}

		region := regionOf(dr, in)
		if region == "" {
			response.Fatal(rsp, errors.Errorf("composed resource %q: cannot determine the AWS region: set spec.forProvider.region on the resource or region in the Function input", name))
			return rsp, nil
		}

		matches, err := r.find(ctx, region, q)
		if err != nil {
			response.Fatal(rsp, errors.Wrapf(err, "composed resource %q: cannot look up %s", name, uri))
			return rsp, nil
		}

		switch len(matches) {
		case 1:
			ext, err := r.externalName(ctx, region, q, matches[0])
			if err != nil {
				response.Fatal(rsp, errors.Wrapf(err, "composed resource %q: cannot get the external name of %s", name, matches[0].ARN))
				return rsp, nil
			}
			log.Info("Adopting existing AWS resource", "region", region, "arn", matches[0].ARN, "externalName", ext)
			response.Normalf(rsp, "Composed resource %q adopts existing AWS resource %q", name, ext)
			setExternalName(dr, ext)
		case 0:
			if in.OnNotFound == v1beta1.NotFoundPolicyCreate {
				log.Info("No AWS resource matches, creating a new one", "region", region)
				response.Normalf(rsp, "No AWS resource in %s matches %s, composed resource %q creates a new one", region, uri, name)
				removeExternalName(dr)
				continue
			}
			response.Fatal(rsp, errors.Errorf("composed resource %q: no AWS resource in %s matches %s", name, region, uri))
			return rsp, nil
		default:
			listed := make([]string, 0, maxListedMatches)
			for _, m := range matches[:min(len(matches), maxListedMatches)] {
				listed = append(listed, m.ARN)
			}
			response.Fatal(rsp, errors.Errorf("composed resource %q: %d AWS resources in %s match %s, refine the tags so that exactly one matches: %s",
				name, len(matches), region, uri, strings.Join(listed, ", ")))
			return rsp, nil
		}
	}

	if err := response.SetDesiredComposedResources(rsp, desired); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot set desired composed resources"))
		return rsp, nil
	}

	return rsp, nil
}

// resolver looks up resources and their external names. It creates the
// Finder on first use and caches results, so a request without lookups never
// calls AWS.
type resolver struct {
	newFinder FinderFactory
	req       *fnv1.RunFunctionRequest
	in        *v1beta1.Input

	finder     awsclient.Finder
	matches    map[string][]lookup.Resource
	properties map[string]string
}

// find returns the resources in region that match q, sorted by ARN.
func (r *resolver) find(ctx context.Context, region string, q lookup.Query) ([]lookup.Resource, error) {
	tagsOnly := q
	tagsOnly.Field = ""
	key := region + " " + tagsOnly.String()
	if m, ok := r.matches[key]; ok {
		return m, nil
	}

	f, err := r.getFinder(ctx, region)
	if err != nil {
		return nil, err
	}

	arns, err := f.Find(ctx, region, q)
	if err != nil {
		return nil, err
	}

	var matches []lookup.Resource
	for _, a := range arns {
		res, ok, err := q.Type.Resource(a)
		if err != nil {
			return nil, err
		}
		if ok && !slices.Contains(matches, res) {
			matches = append(matches, res)
		}
	}
	slices.SortFunc(matches, func(a, b lookup.Resource) int { return strings.Compare(a.ARN, b.ARN) })

	r.matches[key] = matches
	return matches, nil
}

// externalName returns the external name of res. It reads a Cloud Control
// property if the query or the resource type asks for one.
func (r *resolver) externalName(ctx context.Context, region string, q lookup.Query, res lookup.Resource) (string, error) {
	field := q.ExternalNameField()
	if field == "" {
		return q.Type.ExternalName(res), nil
	}

	key := region + " " + res.ARN
	props, ok := r.properties[key]
	if !ok {
		f, err := r.getFinder(ctx, region)
		if err != nil {
			return "", err
		}
		props, err = f.Properties(ctx, region, q.Type.CloudFormationType, q.Type.CloudControlIdentifier(res))
		if err != nil {
			return "", err
		}
		r.properties[key] = props
	}

	return lookup.Field(props, field)
}

func (r *resolver) getFinder(ctx context.Context, region string) (awsclient.Finder, error) {
	if r.finder != nil {
		return r.finder, nil
	}

	o := awsclient.Options{AssumeRoleARN: r.in.AssumeRoleARN, Region: region}
	if _, ok := r.req.GetCredentials()[r.in.CredentialsName]; ok {
		c, err := request.GetCredentials(r.req, r.in.CredentialsName)
		if err != nil {
			return nil, err
		}
		o.Credentials = c.Data
	}

	f, err := r.newFinder(ctx, o)
	if err != nil {
		return nil, err
	}
	r.finder = f
	return f, nil
}

func observedExternalName(observed map[resource.Name]resource.ObservedComposed, name resource.Name) string {
	oc, ok := observed[name]
	if !ok || oc.Resource == nil {
		return ""
	}
	ext := oc.Resource.GetAnnotations()[AnnotationExternalName]
	if lookup.IsURI(ext) {
		return ""
	}
	return ext
}

func regionOf(dr *resource.DesiredComposed, in *v1beta1.Input) string {
	if r, err := dr.Resource.GetString("spec.forProvider.region"); err == nil && r != "" {
		return r
	}
	return in.Region
}

func setExternalName(dr *resource.DesiredComposed, name string) {
	a := dr.Resource.GetAnnotations()
	if a == nil {
		a = map[string]string{}
	}
	a[AnnotationExternalName] = name
	dr.Resource.SetAnnotations(a)
}

func removeExternalName(dr *resource.DesiredComposed) {
	a := dr.Resource.GetAnnotations()
	delete(a, AnnotationExternalName)
	dr.Resource.SetAnnotations(a)
}
