package lookup_test

import (
	"github.com/giepa/function-aws-lookup/internal/lookup"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ResourceType.ExternalName", func() {
	DescribeTable("returns the external name provider-upjet-aws expects",
		func(service, name, arn, want string) {
			t, ok := lookup.Lookup(service, name)
			Expect(ok).To(BeTrue())
			got, ok, err := t.ExternalName(arn)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(got).To(Equal(want))
		},
		Entry("S3 bucket", "s3", "bucket", "arn:aws:s3:::my-bucket", "my-bucket"),
		Entry("VPC", "ec2", "vpc", "arn:aws:ec2:eu-west-1:123456789012:vpc/vpc-0abc", "vpc-0abc"),
		Entry("subnet", "ec2", "subnet", "arn:aws:ec2:eu-west-1:123456789012:subnet/subnet-0abc", "subnet-0abc"),
		Entry("NAT gateway", "ec2", "nat-gateway", "arn:aws:ec2:eu-west-1:123456789012:natgateway/nat-0abc", "nat-0abc"),
		Entry("RDS cluster", "rds", "cluster", "arn:aws:rds:eu-west-1:123456789012:cluster:orders", "orders"),
		Entry("DynamoDB table", "dynamodb", "table", "arn:aws:dynamodb:eu-west-1:123456789012:table/orders", "orders"),
		Entry("ECR repository with a namespace", "ecr", "repository", "arn:aws:ecr:eu-west-1:123456789012:repository/team/app", "team/app"),
		Entry("Lambda function", "lambda", "function", "arn:aws:lambda:eu-west-1:123456789012:function:handler", "handler"),
		Entry("log group", "logs", "log-group", "arn:aws:logs:eu-west-1:123456789012:log-group:/aws/app:*", "/aws/app"),
		Entry("SNS topic", "sns", "topic", "arn:aws:sns:eu-west-1:123456789012:alerts", "alerts"),
		Entry("secret", "secretsmanager", "secret", "arn:aws:secretsmanager:eu-west-1:123456789012:secret:prod/db-AbCdEf", "arn:aws:secretsmanager:eu-west-1:123456789012:secret:prod/db-AbCdEf"),
		Entry("load balancer", "elb", "load-balancer", "arn:aws:elasticloadbalancing:eu-west-1:123456789012:loadbalancer/app/web/50dc6c495c0c9188", "arn:aws:elasticloadbalancing:eu-west-1:123456789012:loadbalancer/app/web/50dc6c495c0c9188"),
	)

	DescribeTable("rejects ARNs of other kinds of resources",
		func(service, name, arn string) {
			t, ok := lookup.Lookup(service, name)
			Expect(ok).To(BeTrue())
			_, ok, err := t.ExternalName(arn)
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
		_, _, err := t.ExternalName("not-an-arn")
		Expect(err).To(HaveOccurred())
	})
})
