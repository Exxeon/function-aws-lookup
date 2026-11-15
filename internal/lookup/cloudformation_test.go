package lookup_test

import (
	"github.com/Exxeon/function-aws-lookup/internal/lookup"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CloudFormationTypes", func() {
	// These mappings were checked by hand against the CloudFormation schemas.
	// They guard the generator's matching rules when the table is regenerated.
	DescribeTable("returns the CloudFormation type of an ARN first",
		func(arn, want string) {
			types := lookup.CloudFormationTypes(arn)
			Expect(types).NotTo(BeEmpty())
			Expect(types[0]).To(Equal(want))
		},
		Entry(nil, "arn:aws:s3:::my-bucket", "AWS::S3::Bucket"),
		Entry(nil, "arn:aws:s3:eu-west-1:123456789012:accesspoint/ap", "AWS::S3::AccessPoint"),
		Entry(nil, "arn:aws:sns:eu-west-1:123456789012:alerts", "AWS::SNS::Topic"),
		Entry(nil, "arn:aws:sqs:eu-west-1:123456789012:orders", "AWS::SQS::Queue"),
		Entry(nil, "arn:aws:ec2:eu-west-1:123456789012:vpc/vpc-0abc", "AWS::EC2::VPC"),
		Entry(nil, "arn:aws:ec2:eu-west-1:123456789012:subnet/subnet-0abc", "AWS::EC2::Subnet"),
		Entry(nil, "arn:aws:ec2:eu-west-1:123456789012:security-group/sg-0abc", "AWS::EC2::SecurityGroup"),
		Entry(nil, "arn:aws:ec2:eu-west-1:123456789012:instance/i-0abc", "AWS::EC2::Instance"),
		Entry(nil, "arn:aws:ec2:eu-west-1:123456789012:natgateway/nat-0abc", "AWS::EC2::NatGateway"),
		Entry(nil, "arn:aws:ec2:eu-west-1:123456789012:internet-gateway/igw-0abc", "AWS::EC2::InternetGateway"),
		Entry(nil, "arn:aws:ec2:eu-west-1:123456789012:route-table/rtb-0abc", "AWS::EC2::RouteTable"),
		Entry(nil, "arn:aws:rds:eu-west-1:123456789012:db:orders", "AWS::RDS::DBInstance"),
		Entry(nil, "arn:aws:rds:eu-west-1:123456789012:cluster:orders", "AWS::RDS::DBCluster"),
		Entry(nil, "arn:aws:dynamodb:eu-west-1:123456789012:table/orders", "AWS::DynamoDB::Table"),
		Entry(nil, "arn:aws:lambda:eu-west-1:123456789012:function:handler", "AWS::Lambda::Function"),
		Entry(nil, "arn:aws:logs:eu-west-1:123456789012:log-group:/aws/app", "AWS::Logs::LogGroup"),
		Entry(nil, "arn:aws:kms:eu-west-1:123456789012:key/1234abcd-12ab-34cd-56ef-1234567890ab", "AWS::KMS::Key"),
		Entry(nil, "arn:aws:ecr:eu-west-1:123456789012:repository/team/app", "AWS::ECR::Repository"),
		Entry(nil, "arn:aws:eks:eu-west-1:123456789012:cluster/main", "AWS::EKS::Cluster"),
		Entry(nil, "arn:aws:ecs:eu-west-1:123456789012:service/main/web", "AWS::ECS::Service"),
		Entry(nil, "arn:aws:elasticfilesystem:eu-west-1:123456789012:file-system/fs-0abc", "AWS::EFS::FileSystem"),
		Entry(nil, "arn:aws:elasticache:eu-west-1:123456789012:replicationgroup:orders", "AWS::ElastiCache::ReplicationGroup"),
		Entry(nil, "arn:aws:elasticloadbalancing:eu-west-1:123456789012:targetgroup/web/50dc6c495c0c9188", "AWS::ElasticLoadBalancingV2::TargetGroup"),
		Entry(nil, "arn:aws:secretsmanager:eu-west-1:123456789012:secret:prod/db-AbCdEf", "AWS::SecretsManager::Secret"),
		Entry(nil, "arn:aws:iam::123456789012:role/service/adopt", "AWS::IAM::Role"),
		Entry(nil, "arn:aws:states:eu-west-1:123456789012:stateMachine:orders", "AWS::StepFunctions::StateMachine"),
		Entry(nil, "arn:aws-cn:s3:::my-bucket", "AWS::S3::Bucket"),
	)

	It("returns every type that shares an ARN format", func() {
		Expect(lookup.CloudFormationTypes("arn:aws:datasync:eu-west-1:123456789012:location/loc-0abc")).To(ContainElements(
			"AWS::DataSync::LocationS3", "AWS::DataSync::LocationEFS", "AWS::DataSync::LocationNFS"))
	})

	It("returns both load balancer types, which share their IAM actions", func() {
		Expect(lookup.CloudFormationTypes("arn:aws:elasticloadbalancing:eu-west-1:123456789012:loadbalancer/app/web/50dc6c495c0c9188")).To(ConsistOf(
			"AWS::ElasticLoadBalancing::LoadBalancer", "AWS::ElasticLoadBalancingV2::LoadBalancer"))
	})

	It("returns nothing for an unknown ARN", func() {
		Expect(lookup.CloudFormationTypes("arn:aws:nope:eu-west-1:123456789012:thing/x")).To(BeEmpty())
		Expect(lookup.CloudFormationTypes("not-an-arn")).To(BeEmpty())
	})
})
