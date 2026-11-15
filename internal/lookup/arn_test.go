package lookup_test

import (
	"github.com/Exxeon/function-aws-lookup/internal/lookup"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Query.Resource", func() {
	DescribeTable("takes the ID from the ARN",
		func(uri, arn, id string) {
			q, err := lookup.Parse(uri)
			Expect(err).NotTo(HaveOccurred())
			r, ok, err := q.Resource(arn)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(r).To(Equal(lookup.Resource{ARN: arn, ID: id}))
		},
		Entry("S3 bucket", "aws-lookup://s3?a=b", "arn:aws:s3:::my-bucket", "my-bucket"),
		Entry("SNS topic", "aws-lookup://sns?a=b", "arn:aws:sns:eu-west-1:123456789012:alerts", "alerts"),
		Entry("SQS queue", "aws-lookup://sqs?a=b", "arn:aws:sqs:eu-west-1:123456789012:orders", "orders"),
		Entry("VPC", "aws-lookup://ec2/vpc?a=b", "arn:aws:ec2:eu-west-1:123456789012:vpc/vpc-0abc", "vpc-0abc"),
		Entry("RDS instance", "aws-lookup://rds/db?a=b", "arn:aws:rds:eu-west-1:123456789012:db:orders", "orders"),
		Entry("EFS file system", "aws-lookup://elasticfilesystem/file-system?a=b", "arn:aws:elasticfilesystem:eu-west-1:123456789012:file-system/fs-0abc", "fs-0abc"),
		Entry("ECR repository with a namespace", "aws-lookup://ecr/repository?a=b", "arn:aws:ecr:eu-west-1:123456789012:repository/team/app", "team/app"),
		Entry("log group", "aws-lookup://logs/log-group?a=b", "arn:aws:logs:eu-west-1:123456789012:log-group:/aws/app:*", "/aws/app"),
		Entry("ECS service", "aws-lookup://ecs/service?a=b", "arn:aws:ecs:eu-west-1:123456789012:service/main/web", "main/web"),
		Entry("load balancer", "aws-lookup://elasticloadbalancing/loadbalancer?a=b",
			"arn:aws:elasticloadbalancing:eu-west-1:123456789012:loadbalancer/app/web/50dc6c495c0c9188", "app/web/50dc6c495c0c9188"),
		Entry("API Gateway REST API", "aws-lookup://apigateway/restapis?a=b", "arn:aws:apigateway:eu-west-1::/restapis/a1b2c3", "a1b2c3"),
		Entry("IAM role", "aws-lookup://iam/role?a=b", "arn:aws:iam::123456789012:role/service/adopt", "service/adopt"),
	)

	DescribeTable("rejects ARNs of other resources",
		func(uri, arn string) {
			q, err := lookup.Parse(uri)
			Expect(err).NotTo(HaveOccurred())
			_, ok, err := q.Resource(arn)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		},
		Entry("S3 access point when looking for buckets", "aws-lookup://s3?a=b", "arn:aws:s3:eu-west-1:123456789012:accesspoint/ap"),
		Entry("other type", "aws-lookup://rds/db?a=b", "arn:aws:rds:eu-west-1:123456789012:cluster:orders"),
		Entry("type that only starts the same", "aws-lookup://rds/db?a=b", "arn:aws:rds:eu-west-1:123456789012:dbproxy:orders"),
		Entry("other service", "aws-lookup://ec2/vpc?a=b", "arn:aws:s3:::vpc"),
		Entry("no ID", "aws-lookup://ec2/vpc?a=b", "arn:aws:ec2:eu-west-1:123456789012:vpc/"),
	)

	It("returns an error for an invalid ARN", func() {
		q, err := lookup.Parse("aws-lookup://s3?a=b")
		Expect(err).NotTo(HaveOccurred())
		_, _, err = q.Resource("not-an-arn")
		Expect(err).To(HaveOccurred())
	})
})
