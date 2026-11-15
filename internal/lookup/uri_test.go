package lookup_test

import (
	"github.com/giepa/function-aws-lookup/internal/lookup"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parse", func() {
	It("parses the service, type and tags", func() {
		q, err := lookup.Parse("aws-lookup://s3/bucket?tag1=mytag&tag2=othervalue")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.Type.Service).To(Equal("s3"))
		Expect(q.Type.Name).To(Equal("bucket"))
		Expect(q.Tags).To(Equal(map[string][]string{"tag1": {"mytag"}, "tag2": {"othervalue"}}))
	})

	It("collects repeated tags as alternative values", func() {
		q, err := lookup.Parse("aws-lookup://ec2/vpc?env=prod&env=staging")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.Tags).To(Equal(map[string][]string{"env": {"prod", "staging"}}))
	})

	It("treats a tag without a value as any value", func() {
		q, err := lookup.Parse("aws-lookup://ec2/vpc?legacy&team=")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.Tags).To(Equal(map[string][]string{"legacy": nil, "team": nil}))
	})

	It("decodes escaped tag keys and values", func() {
		q, err := lookup.Parse("aws-lookup://s3/bucket?aws%3Acloudformation%3Astack-name=my%20stack")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.Tags).To(Equal(map[string][]string{"aws:cloudformation:stack-name": {"my stack"}}))
	})

	It("parses the field from the fragment", func() {
		q, err := lookup.Parse("aws-lookup://rds/instance?team=payments#Endpoint/Address")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.Tags).To(Equal(map[string][]string{"team": {"payments"}}))
		Expect(q.Field).To(Equal("Endpoint/Address"))
		Expect(q.ExternalNameField()).To(Equal("Endpoint/Address"))
	})

	It("uses the default field of the type when there is no fragment", func() {
		q, err := lookup.Parse("aws-lookup://rds/instance?team=payments")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.Field).To(BeEmpty())
		Expect(q.ExternalNameField()).To(Equal("DbiResourceId"))
	})

	It("needs no field for types whose external name is in the ARN", func() {
		q, err := lookup.Parse("aws-lookup://s3/bucket?team=payments")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.ExternalNameField()).To(BeEmpty())
	})

	DescribeTable("rejects invalid URIs",
		func(uri, msg string) {
			_, err := lookup.Parse(uri)
			Expect(err).To(MatchError(ContainSubstring(msg)))
		},
		Entry("wrong scheme", "https://s3/bucket?a=b", "scheme must be aws-lookup"),
		Entry("missing type", "aws-lookup://s3?a=b", "must have the form"),
		Entry("unsupported type", "aws-lookup://s3/object?a=b", "unsupported resource type s3/object"),
		Entry("no tags", "aws-lookup://s3/bucket", "at least one tag"),
		Entry("empty tag key", "aws-lookup://s3/bucket?=b", "tag keys must not be empty"),
		Entry("fragment before the query", "aws-lookup://s3/bucket#BucketName?a=b", "at least one tag"),
		Entry("empty field segment", "aws-lookup://s3/bucket?a=b#Endpoint//Address", "empty path segments"),
	)
})

var _ = Describe("Query.String", func() {
	It("sorts tags so that equal queries give equal strings", func() {
		a, err := lookup.Parse("aws-lookup://s3/bucket?b=2&a=1&c")
		Expect(err).NotTo(HaveOccurred())
		b, err := lookup.Parse("aws-lookup://s3/bucket?c=&a=1&b=2")
		Expect(err).NotTo(HaveOccurred())
		Expect(a.String()).To(Equal("aws-lookup://s3/bucket?a=1&b=2&c="))
		Expect(a.String()).To(Equal(b.String()))
	})

	It("keeps the field", func() {
		q, err := lookup.Parse("aws-lookup://rds/instance?a=1#DbiResourceId")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.String()).To(Equal("aws-lookup://rds/instance?a=1#DbiResourceId"))
	})
})

var _ = Describe("IsURI", func() {
	It("recognises aws-lookup URIs", func() {
		Expect(lookup.IsURI("aws-lookup://s3/bucket?a=b")).To(BeTrue())
		Expect(lookup.IsURI("my-bucket")).To(BeFalse())
		Expect(lookup.IsURI("")).To(BeFalse())
	})
})
