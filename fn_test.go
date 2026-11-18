package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"

	"github.com/Exxeon/function-aws-lookup/input/v1beta1"
	"github.com/Exxeon/function-aws-lookup/internal/awsclient"
	"github.com/Exxeon/function-aws-lookup/internal/lookup"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type findCall struct {
	region string
	query  string
}

type propertiesCall struct {
	region, typeName, identifier string
}

type fakeFinder struct {
	arns  []string
	err   error
	calls []findCall

	// resources maps "<CloudFormation type> <identifier>" to properties.
	resources      map[string]string
	propertiesErr  error
	propertiesCall []propertiesCall
}

func (f *fakeFinder) Find(_ context.Context, region string, q lookup.Query) ([]string, error) {
	f.calls = append(f.calls, findCall{region: region, query: q.String()})
	return f.arns, f.err
}

func (f *fakeFinder) Properties(_ context.Context, region, typeName, identifier string) (string, error) {
	f.propertiesCall = append(f.propertiesCall, propertiesCall{region: region, typeName: typeName, identifier: identifier})
	if f.propertiesErr != nil {
		return "", f.propertiesErr
	}
	if p, ok := f.resources[typeName+" "+identifier]; ok {
		return p, nil
	}
	return "", fmt.Errorf("%w: %s %s", awsclient.ErrWrongType, typeName, identifier)
}

const (
	bucketURI   = "aws-lookup://s3?tag1=mytag&tag2=othervalue"
	bucketProps = `{"BucketName": "legacy-bucket", "DomainName": "legacy-bucket.s3.amazonaws.com"}`
)

func bucket(annotation string) *fnv1.Resource {
	return &fnv1.Resource{Resource: resource.MustStructJSON(`{
		"apiVersion": "s3.aws.m.upbound.io/v1beta1",
		"kind": "Bucket",
		"metadata": {"annotations": {"crossplane.io/external-name": "` + annotation + `"}},
		"spec": {"forProvider": {"region": "eu-west-1"}}
	}`)}
}

// bucketWithPolicy is a bucket with an on-not-found annotation.
func bucketWithPolicy(externalName, policy string) *fnv1.Resource {
	return &fnv1.Resource{Resource: resource.MustStructJSON(`{
		"apiVersion": "s3.aws.m.upbound.io/v1beta1",
		"kind": "Bucket",
		"metadata": {"annotations": {
			"crossplane.io/external-name": "` + externalName + `",
			"awslookup.fn.exxeon.io/on-not-found": "` + policy + `"
		}},
		"spec": {"forProvider": {"region": "eu-west-1"}}
	}`)}
}

func annotation(rsp *fnv1.RunFunctionResponse, name string) (string, bool) {
	r := rsp.GetDesired().GetResources()[name]
	Expect(r).NotTo(BeNil())
	v, ok := r.GetResource().GetFields()["metadata"].GetStructValue().GetFields()["annotations"].GetStructValue().GetFields()[AnnotationExternalName]
	return v.GetStringValue(), ok
}

func externalName(rsp *fnv1.RunFunctionResponse, name string) string {
	v, _ := annotation(rsp, name)
	return v
}

func fatal(rsp *fnv1.RunFunctionResponse) string {
	for _, r := range rsp.GetResults() {
		if r.GetSeverity() == fnv1.Severity_SEVERITY_FATAL {
			return r.GetMessage()
		}
	}
	return ""
}

var _ = Describe("not-found policies", func() {
	DescribeTable("validates supported policy names",
		func(policy string, valid bool) {
			Expect(validPolicy(v1beta1.NotFoundPolicy(policy))).To(Equal(valid))
		},
		Entry("Fail", "Fail", true),
		Entry("Create", "Create", true),
		Entry("Skip", "Skip", true),
		Entry("empty value", "", false),
		Entry("unknown value", "Nope", false),
		Entry("wrong case", "skip", false),
		Entry("surrounding whitespace", " Skip ", false),
	)

	It("lists the supported policies for error messages", func() {
		Expect(policies()).To(Equal("Fail, Create, Skip"))
	})
})

var _ = Describe("RunFunction", func() {
	var (
		finder *fakeFinder
		opts   []awsclient.Options
		fn     *Function
		req    *fnv1.RunFunctionRequest
		newErr error
	)

	BeforeEach(func() {
		finder = &fakeFinder{}
		opts = nil
		newErr = nil
		fn = &Function{
			log: logging.NewNopLogger(),
			newFinder: func(_ context.Context, o awsclient.Options) (awsclient.Finder, error) {
				opts = append(opts, o)
				return finder, newErr
			},
		}
		req = &fnv1.RunFunctionRequest{
			Desired: &fnv1.State{Resources: map[string]*fnv1.Resource{"bucket": bucket(bucketURI)}},
		}
	})

	run := func() *fnv1.RunFunctionResponse {
		rsp, err := fn.RunFunction(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		return rsp
	}

	It("handles a request without desired state without calling AWS", func() {
		req.Desired = nil

		rsp := run()

		Expect(fatal(rsp)).To(BeEmpty())
		Expect(rsp.GetDesired().GetResources()).To(BeEmpty())
		Expect(opts).To(BeEmpty())
		Expect(finder.calls).To(BeEmpty())
	})

	It("injects the external name of the single matching resource", func() {
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}

		rsp := run()

		Expect(fatal(rsp)).To(BeEmpty())
		Expect(externalName(rsp, "bucket")).To(Equal("legacy-bucket"))
		Expect(finder.calls).To(Equal([]findCall{{region: "eu-west-1", query: bucketURI}}))
		Expect(finder.propertiesCall).To(BeEmpty())
	})

	It("reads the field in the fragment with Cloud Control", func() {
		req.Desired.Resources["bucket"] = bucket(bucketURI + "#DomainName")
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}
		finder.resources = map[string]string{"AWS::S3::Bucket legacy-bucket": bucketProps}

		rsp := run()

		Expect(fatal(rsp)).To(BeEmpty())
		Expect(externalName(rsp, "bucket")).To(Equal("legacy-bucket.s3.amazonaws.com"))
		Expect(finder.propertiesCall).To(Equal([]propertiesCall{{region: "eu-west-1", typeName: "AWS::S3::Bucket", identifier: "legacy-bucket"}}))
	})

	It("uses the ID from the ARN without a fragment", func() {
		req.Desired.Resources["bucket"] = bucket("aws-lookup://rds/db?team=payments")
		finder.arns = []string{"arn:aws:rds:eu-west-1:123456789012:db:orders"}

		rsp := run()

		Expect(fatal(rsp)).To(BeEmpty())
		Expect(externalName(rsp, "bucket")).To(Equal("orders"))
		Expect(finder.propertiesCall).To(BeEmpty())
	})

	It("reads the DBI resource ID of an RDS instance", func() {
		req.Desired.Resources["bucket"] = bucket("aws-lookup://rds/db?team=payments#DbiResourceId")
		finder.arns = []string{"arn:aws:rds:eu-west-1:123456789012:db:orders"}
		finder.resources = map[string]string{"AWS::RDS::DBInstance orders": `{"DBInstanceIdentifier": "orders", "DbiResourceId": "db-ABC123"}`}

		rsp := run()

		Expect(fatal(rsp)).To(BeEmpty())
		Expect(externalName(rsp, "bucket")).To(Equal("db-ABC123"))
		Expect(finder.propertiesCall).To(Equal([]propertiesCall{{region: "eu-west-1", typeName: "AWS::RDS::DBInstance", identifier: "orders"}}))
	})

	It("tries the ARN when Cloud Control doesn't know the ID", func() {
		req.Desired.Resources["bucket"] = bucket("aws-lookup://sns?team=payments#TopicName")
		finder.arns = []string{"arn:aws:sns:eu-west-1:123456789012:alerts"}
		finder.resources = map[string]string{"AWS::SNS::Topic arn:aws:sns:eu-west-1:123456789012:alerts": `{"TopicName": "alerts"}`}

		Expect(externalName(run(), "bucket")).To(Equal("alerts"))
		Expect(finder.propertiesCall).To(Equal([]propertiesCall{
			{region: "eu-west-1", typeName: "AWS::SNS::Topic", identifier: "alerts"},
			{region: "eu-west-1", typeName: "AWS::SNS::Topic", identifier: "arn:aws:sns:eu-west-1:123456789012:alerts"},
		}))
	})

	It("tries every CloudFormation type that shares the ARN format", func() {
		req.Desired.Resources["bucket"] = bucket("aws-lookup://datasync/location?team=payments#LocationUri")
		arn := "arn:aws:datasync:eu-west-1:123456789012:location/loc-0abc"
		finder.arns = []string{arn}
		finder.resources = map[string]string{"AWS::DataSync::LocationS3 " + arn: `{"LocationUri": "s3://legacy-bucket/"}`}

		Expect(externalName(run(), "bucket")).To(Equal("s3://legacy-bucket/"))
		Expect(len(finder.propertiesCall)).To(BeNumerically(">", 2))
	})

	It("fails when Cloud Control has no resource for any candidate", func() {
		req.Desired.Resources["bucket"] = bucket(bucketURI + "#DomainName")
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}

		Expect(fatal(run())).To(ContainSubstring("Cloud Control has no resource for ARN arn:aws:s3:::legacy-bucket, tried AWS::S3::Bucket legacy-bucket"))
	})

	It("fails when no CloudFormation type is known for the ARN", func() {
		req.Desired.Resources["bucket"] = bucket("aws-lookup://nope/thing?team=payments#Name")
		finder.arns = []string{"arn:aws:nope:eu-west-1:123456789012:thing/x"}

		Expect(fatal(run())).To(ContainSubstring(`no CloudFormation type is known for ARN arn:aws:nope:eu-west-1:123456789012:thing/x, so fields can't be read; leave out the #field to use "x"`))
	})

	It("fails and lists the available properties when the field is missing", func() {
		req.Desired.Resources["bucket"] = bucket(bucketURI + "#Nope")
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}
		finder.resources = map[string]string{"AWS::S3::Bucket legacy-bucket": bucketProps}

		Expect(fatal(run())).To(ContainSubstring("available properties are BucketName, DomainName"))
	})

	It("fails when Cloud Control fails", func() {
		req.Desired.Resources["bucket"] = bucket(bucketURI + "#DomainName")
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}
		finder.propertiesErr = errors.New("not authorized to perform s3:GetBucketTagging")

		Expect(fatal(run())).To(ContainSubstring("s3:GetBucketTagging"))
	})

	It("keeps the observed external name without calling AWS", func() {
		req.Observed = &fnv1.State{Resources: map[string]*fnv1.Resource{"bucket": bucket("legacy-bucket")}}

		rsp := run()

		Expect(fatal(rsp)).To(BeEmpty())
		Expect(externalName(rsp, "bucket")).To(Equal("legacy-bucket"))
		Expect(opts).To(BeEmpty())
	})

	It("looks up resources whose observed external name is still a URI", func() {
		req.Observed = &fnv1.State{Resources: map[string]*fnv1.Resource{"bucket": bucket(bucketURI)}}
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}

		Expect(externalName(run(), "bucket")).To(Equal("legacy-bucket"))
	})

	It("leaves resources without a lookup URI alone and does not call AWS", func() {
		req.Desired.Resources["bucket"] = bucket("my-bucket")

		rsp := run()

		Expect(externalName(rsp, "bucket")).To(Equal("my-bucket"))
		Expect(opts).To(BeEmpty())
	})

	It("fails when several resources match", func() {
		finder.arns = []string{"arn:aws:s3:::a", "arn:aws:s3:::b"}

		Expect(fatal(run())).To(ContainSubstring("2 AWS resources in eu-west-1 match " + bucketURI + ", refine the tags so that exactly one matches: arn:aws:s3:::a, arn:aws:s3:::b"))
	})

	It("ignores matches of other kinds of resources", func() {
		finder.arns = []string{"arn:aws:s3:::legacy-bucket", "arn:aws:s3:eu-west-1:123456789012:accesspoint/ap"}

		Expect(externalName(run(), "bucket")).To(Equal("legacy-bucket"))
	})

	It("fails by default when nothing matches", func() {
		Expect(fatal(run())).To(ContainSubstring("no AWS resource in eu-west-1 matches"))
	})

	It("removes the annotation when nothing matches and onNotFound is Create", func() {
		req.Input = resource.MustStructJSON(`{"apiVersion": "awslookup.fn.exxeon.io/v1beta1", "kind": "Input", "onNotFound": "Create"}`)

		rsp := run()

		Expect(fatal(rsp)).To(BeEmpty())
		_, ok := annotation(rsp, "bucket")
		Expect(ok).To(BeFalse())
	})

	It("omits the unmatched resource and keeps other resources when onNotFound is Skip", func() {
		req.Input = resource.MustStructJSON(`{"apiVersion": "awslookup.fn.exxeon.io/v1beta1", "kind": "Input", "onNotFound": "Skip"}`)
		req.Desired.Resources["other"] = bucket("other-bucket")

		rsp := run()

		Expect(fatal(rsp)).To(BeEmpty())
		Expect(rsp.GetDesired().GetResources()).NotTo(HaveKey("bucket"))
		Expect(externalName(rsp, "other")).To(Equal("other-bucket"))
		Expect(finder.calls).To(Equal([]findCall{{region: "eu-west-1", query: bucketURI}}))
	})

	It("keeps an already-adopted observed resource when onNotFound is Skip", func() {
		req.Input = resource.MustStructJSON(`{"apiVersion": "awslookup.fn.exxeon.io/v1beta1", "kind": "Input", "onNotFound": "Skip"}`)
		req.Observed = &fnv1.State{Resources: map[string]*fnv1.Resource{"bucket": bucket("legacy-bucket")}}

		rsp := run()

		Expect(fatal(rsp)).To(BeEmpty())
		Expect(externalName(rsp, "bucket")).To(Equal("legacy-bucket"))
		Expect(opts).To(BeEmpty())
		Expect(finder.calls).To(BeEmpty())
	})

	DescribeTable("does not drop an observed resource with an unresolved external name when onNotFound is Skip",
		func(observedName string) {
			req.Input = resource.MustStructJSON(`{"apiVersion": "awslookup.fn.exxeon.io/v1beta1", "kind": "Input", "onNotFound": "Skip"}`)
			req.Observed = &fnv1.State{Resources: map[string]*fnv1.Resource{"bucket": bucket(observedName)}}

			rsp := run()

			Expect(fatal(rsp)).To(ContainSubstring(`composed resource "bucket": already exists without an external name and no AWS resource in eu-west-1 matches`))
			Expect(rsp.GetDesired().GetResources()).To(HaveKey("bucket"))
			Expect(finder.calls).To(Equal([]findCall{{region: "eu-west-1", query: bucketURI}}))
		},
		Entry("empty external name", ""),
		Entry("lookup URI", bucketURI),
	)

	Describe("the on-not-found annotation", func() {
		const createInput = `{"apiVersion": "awslookup.fn.exxeon.io/v1beta1", "kind": "Input", "onNotFound": "Create"}`

		It("creates the resource when it says Create and the input says Fail", func() {
			req.Desired.Resources["bucket"] = bucketWithPolicy(bucketURI, "Create")

			rsp := run()

			Expect(fatal(rsp)).To(BeEmpty())
			_, ok := annotation(rsp, "bucket")
			Expect(ok).To(BeFalse())
		})

		It("skips the resource when it says Skip and the input says Fail", func() {
			req.Desired.Resources["bucket"] = bucketWithPolicy(bucketURI, "Skip")
			req.Desired.Resources["other"] = bucket("other-bucket")

			rsp := run()

			Expect(fatal(rsp)).To(BeEmpty())
			Expect(rsp.GetDesired().GetResources()).NotTo(HaveKey("bucket"))
			Expect(externalName(rsp, "other")).To(Equal("other-bucket"))
		})

		It("fails when it says Fail and the input says Create", func() {
			req.Desired.Resources["bucket"] = bucketWithPolicy(bucketURI, "Fail")
			req.Input = resource.MustStructJSON(createInput)

			Expect(fatal(run())).To(ContainSubstring(`composed resource "bucket": no AWS resource in eu-west-1 matches`))
		})

		It("applies to its own resource only", func() {
			req.Desired.Resources["bucket"] = bucketWithPolicy(bucketURI, "Create")
			req.Desired.Resources["other"] = bucket("aws-lookup://sqs?team=payments")

			Expect(fatal(run())).To(ContainSubstring(`composed resource "other": no AWS resource in eu-west-1 matches`))
		})

		It("stays on the resource", func() {
			req.Desired.Resources["bucket"] = bucketWithPolicy(bucketURI, "Create")

			r := run().GetDesired().GetResources()["bucket"]
			v := r.GetResource().GetFields()["metadata"].GetStructValue().GetFields()["annotations"].GetStructValue().GetFields()[AnnotationOnNotFound]
			Expect(v.GetStringValue()).To(Equal("Create"))
		})

		It("doesn't change what happens when several resources match", func() {
			req.Desired.Resources["bucket"] = bucketWithPolicy(bucketURI, "Create")
			finder.arns = []string{"arn:aws:s3:::a", "arn:aws:s3:::b"}

			Expect(fatal(run())).To(ContainSubstring("2 AWS resources in eu-west-1 match"))
		})

		It("is ignored on resources without a lookup URI", func() {
			req.Desired.Resources["bucket"] = bucketWithPolicy("my-bucket", "Nope")

			Expect(fatal(run())).To(BeEmpty())
		})

		It("fails on an invalid value", func() {
			req.Desired.Resources["bucket"] = bucketWithPolicy(bucketURI, "create")

			Expect(fatal(run())).To(ContainSubstring(`composed resource "bucket": invalid awslookup.fn.exxeon.io/on-not-found annotation "create", must be one of Fail, Create, Skip`))
		})

		It("fails on an invalid value also when the observed external name is kept", func() {
			req.Desired.Resources["bucket"] = bucketWithPolicy(bucketURI, "Maybe")
			req.Observed = &fnv1.State{Resources: map[string]*fnv1.Resource{"bucket": bucket("legacy-bucket")}}

			Expect(fatal(run())).To(ContainSubstring(`invalid awslookup.fn.exxeon.io/on-not-found annotation "Maybe"`))
			Expect(opts).To(BeEmpty())
		})
	})

	It("fails on an invalid onNotFound in the input", func() {
		req.Input = resource.MustStructJSON(`{"apiVersion": "awslookup.fn.exxeon.io/v1beta1", "kind": "Input", "onNotFound": "Nope"}`)

		Expect(fatal(run())).To(ContainSubstring(`invalid onNotFound "Nope" in the Function input, must be one of Fail, Create, Skip`))
	})

	It("fails when the AWS lookup fails", func() {
		finder.err = errors.New("access denied")

		Expect(fatal(run())).To(ContainSubstring("access denied"))
	})

	It("fails when the finder cannot be created", func() {
		newErr = errors.New("bad credentials")

		Expect(fatal(run())).To(ContainSubstring("bad credentials"))
	})

	It("fails on an invalid URI", func() {
		req.Desired.Resources["bucket"] = bucket("aws-lookup://S3?a=b")

		Expect(fatal(run())).To(ContainSubstring("must have the form"))
	})

	It("uses the input region when the resource has none", func() {
		req.Desired.Resources["bucket"] = &fnv1.Resource{Resource: resource.MustStructJSON(`{
			"apiVersion": "s3.aws.m.upbound.io/v1beta1",
			"kind": "Bucket",
			"metadata": {"annotations": {"crossplane.io/external-name": "` + bucketURI + `"}}
		}`)}
		req.Input = resource.MustStructJSON(`{"apiVersion": "awslookup.fn.exxeon.io/v1beta1", "kind": "Input", "region": "eu-central-1"}`)
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}

		run()

		Expect(finder.calls).To(Equal([]findCall{{region: "eu-central-1", query: bucketURI}}))
	})

	It("fails when no region is known", func() {
		req.Desired.Resources["bucket"] = &fnv1.Resource{Resource: resource.MustStructJSON(`{
			"apiVersion": "s3.aws.m.upbound.io/v1beta1",
			"kind": "Bucket",
			"metadata": {"annotations": {"crossplane.io/external-name": "` + bucketURI + `"}}
		}`)}

		Expect(fatal(run())).To(ContainSubstring("cannot determine the AWS region"))
	})

	It("passes step credentials and the role to assume to the finder", func() {
		req.Credentials = map[string]*fnv1.Credentials{
			"aws-creds": {Source: &fnv1.Credentials_CredentialData{CredentialData: &fnv1.CredentialData{
				Data: map[string][]byte{"credentials": []byte("[default]\naws_access_key_id=AKID\naws_secret_access_key=secret\n")},
			}}},
		}
		req.Input = resource.MustStructJSON(`{"apiVersion": "awslookup.fn.exxeon.io/v1beta1", "kind": "Input", "assumeRoleARN": "arn:aws:iam::123456789012:role/adopt"}`)
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}

		run()

		Expect(opts).To(HaveLen(1))
		Expect(opts[0].AssumeRoleARN).To(Equal("arn:aws:iam::123456789012:role/adopt"))
		Expect(opts[0].Credentials).To(HaveKey("credentials"))
	})

	It("uses the default credential chain when the step has no credentials", func() {
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}

		run()

		Expect(opts).To(HaveLen(1))
		Expect(opts[0].Credentials).To(BeNil())
	})

	It("looks up identical queries once per request", func() {
		req.Desired.Resources["other"] = bucket("aws-lookup://s3?tag2=othervalue&tag1=mytag")
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}

		rsp := run()

		Expect(finder.calls).To(HaveLen(1))
		Expect(externalName(rsp, "other")).To(Equal("legacy-bucket"))
	})

	It("reads the properties of a resource once per request", func() {
		req.Desired.Resources["bucket"] = bucket(bucketURI + "#BucketName")
		req.Desired.Resources["other"] = bucket(bucketURI + "#DomainName")
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}
		finder.resources = map[string]string{"AWS::S3::Bucket legacy-bucket": bucketProps}

		rsp := run()

		Expect(finder.calls).To(HaveLen(1))
		Expect(finder.propertiesCall).To(HaveLen(1))
		Expect(externalName(rsp, "bucket")).To(Equal("legacy-bucket"))
		Expect(externalName(rsp, "other")).To(Equal("legacy-bucket.s3.amazonaws.com"))
	})
})
