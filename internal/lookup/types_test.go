package lookup_test

import (
	"strings"

	"github.com/giepa/function-aws-lookup/internal/lookup"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ResourceType", func() {
	DescribeTable("returns the external name and Cloud Control identifier provider-upjet-aws and Cloud Control expect",
		func(service, name, arn, externalName, identifier string) {
			t, ok := lookup.Lookup(service, name)
			Expect(ok).To(BeTrue())
			r, ok, err := t.Resource(arn)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(r.ARN).To(Equal(arn))
			Expect(t.ExternalName(r)).To(Equal(externalName))
			Expect(t.CloudControlIdentifier(r)).To(Equal(identifier))
		},
		Entry("S3 bucket", "s3", "bucket", "arn:aws:s3:::my-bucket", "my-bucket", "my-bucket"),
		Entry("VPC", "ec2", "vpc", "arn:aws:ec2:eu-west-1:123456789012:vpc/vpc-0abc", "vpc-0abc", "vpc-0abc"),
		Entry("subnet", "ec2", "subnet", "arn:aws:ec2:eu-west-1:123456789012:subnet/subnet-0abc", "subnet-0abc", "subnet-0abc"),
		Entry("NAT gateway", "ec2", "nat-gateway", "arn:aws:ec2:eu-west-1:123456789012:natgateway/nat-0abc", "nat-0abc", "nat-0abc"),
		Entry("RDS cluster", "rds", "cluster", "arn:aws:rds:eu-west-1:123456789012:cluster:orders", "orders", "orders"),
		Entry("RDS instance", "rds", "instance", "arn:aws:rds:eu-west-1:123456789012:db:orders", "orders", "orders"),
		Entry("DynamoDB table", "dynamodb", "table", "arn:aws:dynamodb:eu-west-1:123456789012:table/orders", "orders", "orders"),
		Entry("ECR repository with a namespace", "ecr", "repository", "arn:aws:ecr:eu-west-1:123456789012:repository/team/app", "team/app", "team/app"),
		Entry("Lambda function", "lambda", "function", "arn:aws:lambda:eu-west-1:123456789012:function:handler", "handler", "handler"),
		Entry("log group", "logs", "log-group", "arn:aws:logs:eu-west-1:123456789012:log-group:/aws/app:*", "/aws/app", "/aws/app"),
		Entry("SNS topic", "sns", "topic", "arn:aws:sns:eu-west-1:123456789012:alerts", "alerts", "arn:aws:sns:eu-west-1:123456789012:alerts"),
		Entry("secret", "secretsmanager", "secret", "arn:aws:secretsmanager:eu-west-1:123456789012:secret:prod/db-AbCdEf",
			"arn:aws:secretsmanager:eu-west-1:123456789012:secret:prod/db-AbCdEf", "arn:aws:secretsmanager:eu-west-1:123456789012:secret:prod/db-AbCdEf"),
		Entry("load balancer", "elb", "load-balancer", "arn:aws:elasticloadbalancing:eu-west-1:123456789012:loadbalancer/app/web/50dc6c495c0c9188",
			"arn:aws:elasticloadbalancing:eu-west-1:123456789012:loadbalancer/app/web/50dc6c495c0c9188", "arn:aws:elasticloadbalancing:eu-west-1:123456789012:loadbalancer/app/web/50dc6c495c0c9188"),
	)

	DescribeTable("rejects ARNs of other kinds of resources",
		func(service, name, arn string) {
			t, ok := lookup.Lookup(service, name)
			Expect(ok).To(BeTrue())
			_, ok, err := t.Resource(arn)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		},
		Entry("S3 access point", "s3", "bucket", "arn:aws:s3:eu-west-1:123456789012:accesspoint/ap"),
		Entry("Lambda function version", "lambda", "function", "arn:aws:lambda:eu-west-1:123456789012:function:handler:3"),
		Entry("classic load balancer", "elb", "load-balancer", "arn:aws:elasticloadbalancing:eu-west-1:123456789012:loadbalancer/web"),
		Entry("other service", "ec2", "vpc", "arn:aws:s3:::vpc/vpc-0abc"),
	)

	It("returns an error for an invalid ARN", func() {
		t, _ := lookup.Lookup("s3", "bucket")
		_, _, err := t.Resource("not-an-arn")
		Expect(err).To(HaveOccurred())
	})

	It("has a Cloud Control type for every resource type", func() {
		for _, s := range lookup.Supported() {
			service, name, _ := strings.Cut(s, "/")
			t, ok := lookup.Lookup(service, name)
			Expect(ok).To(BeTrue())
			Expect(t.CloudFormationType).To(HavePrefix("AWS::"), s)
		}
	})

	It("reads the DBI resource ID of RDS instances by default", func() {
		t, _ := lookup.Lookup("rds", "instance")
		Expect(t.DefaultField).To(Equal("DbiResourceId"))
	})
})
