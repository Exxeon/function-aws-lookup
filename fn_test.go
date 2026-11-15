package main

import (
	"context"
	"errors"

	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"

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

	properties     string
	propertiesErr  error
	propertiesCall []propertiesCall
}

func (f *fakeFinder) Find(_ context.Context, region string, q lookup.Query) ([]string, error) {
	f.calls = append(f.calls, findCall{region: region, query: q.String()})
	return f.arns, f.err
}

func (f *fakeFinder) Properties(_ context.Context, region, typeName, identifier string) (string, error) {
	f.propertiesCall = append(f.propertiesCall, propertiesCall{region: region, typeName: typeName, identifier: identifier})
	return f.properties, f.propertiesErr
}

const bucketURI = "aws-lookup://s3/bucket?tag1=mytag&tag2=othervalue"

func bucket(annotation string) *fnv1.Resource {
	return &fnv1.Resource{Resource: resource.MustStructJSON(`{
		"apiVersion": "s3.aws.m.upbound.io/v1beta1",
		"kind": "Bucket",
		"metadata": {"annotations": {"crossplane.io/external-name": "` + annotation + `"}},
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
		finder.properties = `{"BucketName": "legacy-bucket", "DomainName": "legacy-bucket.s3.amazonaws.com"}`

		rsp := run()

		Expect(fatal(rsp)).To(BeEmpty())
		Expect(externalName(rsp, "bucket")).To(Equal("legacy-bucket.s3.amazonaws.com"))
		Expect(finder.propertiesCall).To(Equal([]propertiesCall{{region: "eu-west-1", typeName: "AWS::S3::Bucket", identifier: "legacy-bucket"}}))
	})

	It("reads the DBI resource ID of an RDS instance without a fragment", func() {
		req.Desired = &fnv1.State{Resources: map[string]*fnv1.Resource{"db": {Resource: resource.MustStructJSON(`{
			"apiVersion": "rds.aws.m.upbound.io/v1beta1",
			"kind": "Instance",
			"metadata": {"annotations": {"crossplane.io/external-name": "aws-lookup://rds/instance?team=payments"}},
			"spec": {"forProvider": {"region": "eu-west-1"}}
		}`)}}}
		finder.arns = []string{"arn:aws:rds:eu-west-1:123456789012:db:orders"}
		finder.properties = `{"DBInstanceIdentifier": "orders", "DbiResourceId": "db-ABC123"}`

		rsp := run()

		Expect(fatal(rsp)).To(BeEmpty())
		Expect(externalName(rsp, "db")).To(Equal("db-ABC123"))
		Expect(finder.propertiesCall).To(Equal([]propertiesCall{{region: "eu-west-1", typeName: "AWS::RDS::DBInstance", identifier: "orders"}}))
	})

	It("uses the ARN as the Cloud Control identifier when the type needs it", func() {
		req.Desired.Resources["bucket"] = bucket("aws-lookup://sns/topic?team=payments#TopicName")
		finder.arns = []string{"arn:aws:sns:eu-west-1:123456789012:alerts"}
		finder.properties = `{"TopicArn": "arn:aws:sns:eu-west-1:123456789012:alerts", "TopicName": "alerts"}`

		Expect(externalName(run(), "bucket")).To(Equal("alerts"))
		Expect(finder.propertiesCall[0].identifier).To(Equal("arn:aws:sns:eu-west-1:123456789012:alerts"))
	})

	It("fails and lists the available properties when the field is missing", func() {
		req.Desired.Resources["bucket"] = bucket(bucketURI + "#Nope")
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}
		finder.properties = `{"BucketName": "legacy-bucket", "DomainName": "legacy-bucket.s3.amazonaws.com"}`

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

	It("fails when the AWS lookup fails", func() {
		finder.err = errors.New("access denied")

		Expect(fatal(run())).To(ContainSubstring("access denied"))
	})

	It("fails when the finder cannot be created", func() {
		newErr = errors.New("bad credentials")

		Expect(fatal(run())).To(ContainSubstring("bad credentials"))
	})

	It("fails on an invalid URI", func() {
		req.Desired.Resources["bucket"] = bucket("aws-lookup://s3/object?a=b")

		Expect(fatal(run())).To(ContainSubstring("unsupported resource type"))
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
		req.Desired.Resources["other"] = bucket("aws-lookup://s3/bucket?tag2=othervalue&tag1=mytag")
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}

		rsp := run()

		Expect(finder.calls).To(HaveLen(1))
		Expect(externalName(rsp, "other")).To(Equal("legacy-bucket"))
	})

	It("reads the properties of a resource once per request", func() {
		req.Desired.Resources["bucket"] = bucket(bucketURI + "#BucketName")
		req.Desired.Resources["other"] = bucket(bucketURI + "#DomainName")
		finder.arns = []string{"arn:aws:s3:::legacy-bucket"}
		finder.properties = `{"BucketName": "legacy-bucket", "DomainName": "legacy-bucket.s3.amazonaws.com"}`

		rsp := run()

		Expect(finder.calls).To(HaveLen(1))
		Expect(finder.propertiesCall).To(HaveLen(1))
		Expect(externalName(rsp, "bucket")).To(Equal("legacy-bucket"))
		Expect(externalName(rsp, "other")).To(Equal("legacy-bucket.s3.amazonaws.com"))
	})
})
