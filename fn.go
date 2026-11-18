package main

import (
	"context"
	stderrors "errors"
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

const (
	// AnnotationExternalName is the Crossplane external name annotation.
	AnnotationExternalName = "crossplane.io/external-name"

	// AnnotationOnNotFound sets the not-found policy of one composed
	// resource. It overrides onNotFound in the Function input.
	AnnotationOnNotFound = "awslookup.fn.exxeon.io/on-not-found"
)

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

	in, err := getInput(req)
	if err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
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

		// Check the policy before anything else, so that a typo in the
		// annotation shows up also for resources that are already adopted.
		onNotFound, err := notFoundPolicy(dr, in)
		if err != nil {
			response.Fatal(rsp, errors.Wrapf(err, "composed resource %q", name))
			return rsp, nil
		}

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
			if err := handleNotFound(log, rsp, desired, observed, name, region, onNotFound); err != nil {
				response.Fatal(rsp, err)
				return rsp, nil
			}
		default:
			response.Fatal(rsp, tooManyMatchesError(name, region, uri, matches))
			return rsp, nil
		}
	}

	if rsp.GetDesired() != nil {
		rsp.Desired.Resources = nil
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
		res, ok, err := q.Resource(a)
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

// externalName returns the external name of res: the ID from its ARN, or the
// Cloud Control property named in the URI.
func (r *resolver) externalName(ctx context.Context, region string, q lookup.Query, res lookup.Resource) (string, error) {
	if q.Field == "" {
		return res.ID, nil
	}

	props, ok := r.properties[res.ARN]
	if !ok {
		var err error
		props, err = r.readProperties(ctx, region, res)
		if err != nil {
			return "", err
		}
		r.properties[res.ARN] = props
	}

	return lookup.Field(props, q.Field)
}

// readProperties reads a resource with Cloud Control. Several CloudFormation
// types can share an ARN format, and the Cloud Control identifier is either
// the ID or the ARN, so it tries each combination until one exists.
func (r *resolver) readProperties(ctx context.Context, region string, res lookup.Resource) (string, error) {
	types := lookup.CloudFormationTypes(res.ARN)
	if len(types) == 0 {
		return "", errors.Errorf("no CloudFormation type is known for ARN %s, so fields can't be read; leave out the #field to use %q", res.ARN, res.ID)
	}

	f, err := r.getFinder(ctx, region)
	if err != nil {
		return "", err
	}

	identifiers := []string{res.ID}
	if res.ARN != res.ID {
		identifiers = append(identifiers, res.ARN)
	}

	var tried []string
	for _, t := range types {
		for _, id := range identifiers {
			props, err := f.Properties(ctx, region, t, id)
			if err == nil {
				return props, nil
			}
			if !stderrors.Is(err, awsclient.ErrWrongType) {
				return "", err
			}
			tried = append(tried, t+" "+id)
		}
	}
	return "", errors.Errorf("Cloud Control has no resource for ARN %s, tried %s", res.ARN, strings.Join(tried, ", "))
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

func handleNotFound(
	log logging.Logger,
	rsp *fnv1.RunFunctionResponse,
	desired map[resource.Name]*resource.DesiredComposed,
	observed map[resource.Name]resource.ObservedComposed,
	name resource.Name,
	region string,
	policy v1beta1.NotFoundPolicy,
) error {
	dr := desired[name]
	uri := dr.Resource.GetAnnotations()[AnnotationExternalName]

	switch policy {
	case v1beta1.NotFoundPolicyCreate:
		log.Info("No AWS resource matches, creating a new one", "region", region)
		response.Normalf(rsp, "No AWS resource in %s matches %s, composed resource %q creates a new one", region, uri, name)
		removeExternalName(dr)
	case v1beta1.NotFoundPolicySkip:
		if _, ok := observed[name]; ok {
			return errors.Errorf("composed resource %q: already exists without an external name and no AWS resource in %s matches %s", name, region, uri)
		}
		log.Info("No AWS resource matches, skipping", "region", region)
		response.Normalf(rsp, "No AWS resource in %s matches %s, composed resource %q skips (NotFoundPolicySkip)", region, uri, name)
		delete(desired, name)
	case v1beta1.NotFoundPolicyFail:
		return errors.Errorf("composed resource %q: no AWS resource in %s matches %s", name, region, uri)
	}
	return nil
}

func tooManyMatchesError(name resource.Name, region, uri string, matches []lookup.Resource) error {
	listed := make([]string, 0, maxListedMatches)
	for _, m := range matches[:min(len(matches), maxListedMatches)] {
		listed = append(listed, m.ARN)
	}
	return errors.Errorf("composed resource %q: %d AWS resources in %s match %s, refine the tags so that exactly one matches: %s",
		name, len(matches), region, uri, strings.Join(listed, ", "))
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

// getInput returns the Function input with defaults applied.
func getInput(req *fnv1.RunFunctionRequest) (*v1beta1.Input, error) {
	in := &v1beta1.Input{}
	if req.GetInput() != nil {
		if err := request.GetInput(req, in); err != nil {
			return nil, errors.Wrapf(err, "cannot get Function input from %T", req)
		}
	}
	if in.CredentialsName == "" {
		in.CredentialsName = defaultCredentialsName
	}
	if in.OnNotFound == "" {
		in.OnNotFound = v1beta1.NotFoundPolicyFail
	}
	if !validPolicy(in.OnNotFound) {
		return nil, errors.Errorf("invalid onNotFound %q in the Function input, must be one of %s", in.OnNotFound, policies())
	}
	return in, nil
}

// notFoundPolicy returns the not-found policy of a composed resource: its
// annotation if it has one, otherwise the Function input.
func notFoundPolicy(dr *resource.DesiredComposed, in *v1beta1.Input) (v1beta1.NotFoundPolicy, error) {
	v, ok := dr.Resource.GetAnnotations()[AnnotationOnNotFound]
	if !ok {
		return in.OnNotFound, nil
	}
	p := v1beta1.NotFoundPolicy(v)
	if !validPolicy(p) {
		return "", errors.Errorf("invalid %s annotation %q, must be one of %s", AnnotationOnNotFound, v, policies())
	}
	return p, nil
}

func supportedPolicies() []string {
	return []string{
		string(v1beta1.NotFoundPolicyFail),
		string(v1beta1.NotFoundPolicyCreate),
		string(v1beta1.NotFoundPolicySkip),
	}
}

func validPolicy(p v1beta1.NotFoundPolicy) bool {
	return slices.Contains(supportedPolicies(), string(p))
}

func policies() string {
	return strings.Join(supportedPolicies(), ", ")
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
