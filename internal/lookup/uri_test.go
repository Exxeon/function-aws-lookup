package lookup_test

import (
	"github.com/Exxeon/function-aws-lookup/internal/lookup"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parse", func() {
	It("parses the service, type and tags", func() {
		q, err := lookup.Parse("aws-lookup://rds/db?tag1=mytag&tag2=othervalue")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.Service).To(Equal("rds"))
		Expect(q.Type).To(Equal("db"))
		Expect(q.Filter()).To(Equal("rds:db"))
		Expect(q.Tags).To(Equal(map[string][]string{"tag1": {"mytag"}, "tag2": {"othervalue"}}))
		Expect(q.Field).To(BeEmpty())
	})

	It("accepts a service without a type", func() {
		q, err := lookup.Parse("aws-lookup://s3?team=payments")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.Service).To(Equal("s3"))
		Expect(q.Type).To(BeEmpty())
		Expect(q.Filter()).To(Equal("s3"))
	})

	It("accepts any service and type, also ones that have no CloudFormation type", func() {
		q, err := lookup.Parse("aws-lookup://workspaces-web/ipAccessSettings?team=payments")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.Filter()).To(Equal("workspaces-web:ipAccessSettings"))
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
		q, err := lookup.Parse("aws-lookup://s3?aws%3Acloudformation%3Astack-name=my%20stack")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.Tags).To(Equal(map[string][]string{"aws:cloudformation:stack-name": {"my stack"}}))
	})

	It("parses the field from the fragment", func() {
		q, err := lookup.Parse("aws-lookup://rds/db?team=payments#Endpoint/Address")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.Tags).To(Equal(map[string][]string{"team": {"payments"}}))
		Expect(q.Field).To(Equal("Endpoint/Address"))
	})

	DescribeTable("rejects invalid URIs",
		func(uri, msg string) {
			_, err := lookup.Parse(uri)
			Expect(err).To(MatchError(ContainSubstring(msg)))
		},
		Entry("wrong scheme", "https://s3?a=b", "scheme must be aws-lookup"),
		Entry("no service", "aws-lookup:///db?a=b", "must have the form"),
		Entry("upper case service", "aws-lookup://RDS/db?a=b", "must have the form"),
		Entry("nested type", "aws-lookup://rds/db/orders?a=b", "must have the form"),
		Entry("no tags", "aws-lookup://s3", "at least one tag"),
		Entry("empty tag key", "aws-lookup://s3?=b", "tag keys must not be empty"),
		Entry("fragment before the query", "aws-lookup://s3#BucketName?a=b", "at least one tag"),
		Entry("empty field segment", "aws-lookup://s3?a=b#Endpoint//Address", "empty path segments"),
	)
})

var _ = Describe("Query.String", func() {
	It("sorts tags so that equal queries give equal strings", func() {
		a, err := lookup.Parse("aws-lookup://rds/db?b=2&a=1&c")
		Expect(err).NotTo(HaveOccurred())
		b, err := lookup.Parse("aws-lookup://rds/db?c=&a=1&b=2")
		Expect(err).NotTo(HaveOccurred())
		Expect(a.String()).To(Equal("aws-lookup://rds/db?a=1&b=2&c="))
		Expect(a.String()).To(Equal(b.String()))
	})

	It("keeps the field and leaves out an empty type", func() {
		q, err := lookup.Parse("aws-lookup://s3?a=1#DomainName")
		Expect(err).NotTo(HaveOccurred())
		Expect(q.String()).To(Equal("aws-lookup://s3?a=1#DomainName"))
	})
})

var _ = Describe("IsURI", func() {
	It("recognises aws-lookup URIs", func() {
		Expect(lookup.IsURI("aws-lookup://s3?a=b")).To(BeTrue())
		Expect(lookup.IsURI("my-bucket")).To(BeFalse())
		Expect(lookup.IsURI("")).To(BeFalse())
	})
})
