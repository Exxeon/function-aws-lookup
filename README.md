# function-aws-lookup

A [Crossplane composition function][functions] that adopts existing AWS
resources by tag.

Teams often have AWS resources that were created outside Crossplane, with
Terraform, CDK, CloudFormation or by hand. To bring such a resource under
Crossplane, its managed resource needs the resource's identifier in the
`crossplane.io/external-name` annotation. This function finds that identifier
for you. You write a lookup URI in the annotation, and the function replaces it
with the identifier of the one AWS resource whose tags match:

```yaml
apiVersion: s3.aws.m.upbound.io/v1beta1
kind: Bucket
metadata:
  annotations:
    crossplane.io/external-name: aws-lookup://s3/bucket?team=payments&environment=prod
spec:
  forProvider:
    region: eu-west-1
```

becomes

```yaml
metadata:
  annotations:
    crossplane.io/external-name: payments-prod-7f3a1c
```

If the resources follow the organisation's tagging convention, adopting them
needs no list of names or IDs.

## How it works

For every desired composed resource whose external name starts with
`aws-lookup://`, the function:

1. Keeps the observed external name if the resource already exists and has
   one. The function only calls AWS when a resource is adopted for the first
   time, and it never changes the external name of a resource that already
   exists.
2. Otherwise it searches the resource's region with the
   [Resource Groups Tagging API][tagging-api].
3. If exactly one resource matches, it writes that resource's external name to
   the annotation. If the URI names a field, or the type needs one, it first
   reads the resource with the [Cloud Control API][cloud-control] and takes
   the external name from that field.
4. If several resources match, it stops the pipeline with a fatal result that
   lists them. Refine the tags so that only one matches.
5. If nothing matches, it stops the pipeline (`onNotFound: Fail`, the default)
   or removes the annotation so that the provider creates a new resource
   (`onNotFound: Create`).

The function stops the pipeline whenever it can't resolve a URI. Otherwise
Crossplane would pass the URI to the provider as if it were a resource name.

Put the function after the steps that compose your resources.

## URI format

```
aws-lookup://<service>/<type>?<tag>=<value>&<tag>=<value>[#<field>]
```

- All tags must match.
- Repeat a tag to accept several values: `env=prod&env=production`.
- Leave the value out to match any value: `legacy` or `legacy=`.
- URL-encode special characters: `aws%3Acloudformation%3Astack-name=orders`.
- The optional `#<field>` goes at the end, after the tags. See
  [Choose the field](#choose-the-field).

Supported types, with the external names that
[provider-upjet-aws][provider-aws] expects:

| URI | External name without a field | CloudFormation type |
| --- | --- | --- |
| `dynamodb/table` | table name | `AWS::DynamoDB::Table` |
| `ec2/instance` | instance ID | `AWS::EC2::Instance` |
| `ec2/internet-gateway` | internet gateway ID | `AWS::EC2::InternetGateway` |
| `ec2/nat-gateway` | NAT gateway ID | `AWS::EC2::NatGateway` |
| `ec2/route-table` | route table ID | `AWS::EC2::RouteTable` |
| `ec2/security-group` | security group ID | `AWS::EC2::SecurityGroup` |
| `ec2/subnet` | subnet ID | `AWS::EC2::Subnet` |
| `ec2/vpc` | VPC ID | `AWS::EC2::VPC` |
| `ecr/repository` | repository name | `AWS::ECR::Repository` |
| `efs/file-system` | file system ID | `AWS::EFS::FileSystem` |
| `eks/cluster` | cluster name | `AWS::EKS::Cluster` |
| `elasticache/replication-group` | replication group ID | `AWS::ElastiCache::ReplicationGroup` |
| `elb/load-balancer` | ARN (application, network and gateway load balancers) | `AWS::ElasticLoadBalancingV2::LoadBalancer` |
| `elb/target-group` | ARN | `AWS::ElasticLoadBalancingV2::TargetGroup` |
| `kms/key` | key ID | `AWS::KMS::Key` |
| `lambda/function` | function name | `AWS::Lambda::Function` |
| `logs/log-group` | log group name | `AWS::Logs::LogGroup` |
| `rds/cluster` | cluster identifier | `AWS::RDS::DBCluster` |
| `rds/instance` | `DbiResourceId` field, read with Cloud Control | `AWS::RDS::DBInstance` |
| `s3/bucket` | bucket name | `AWS::S3::Bucket` |
| `secretsmanager/secret` | ARN | `AWS::SecretsManager::Secret` |
| `sns/topic` | topic name | `AWS::SNS::Topic` |

For most types the external name is part of the ARN, so the function needs
only the Tagging API. RDS instances are the exception: since Terraform AWS
provider v5 their external name is the DBI resource ID (`db-ABC123...`), which
is not in the ARN, so the function always reads it with Cloud Control.

To add a type, check its external name configuration in
[provider-upjet-aws][provider-aws-externalname], and the primary identifier of
its CloudFormation type, then add it to `internal/lookup/types.go`. Types that
the Tagging API doesn't support, such as IAM roles, can't be looked up.

## Choose the field

The field that a provider uses as the external name differs per type, and can
change between provider versions. When the default is wrong, name the field
yourself:

```
aws-lookup://rds/instance?team=payments&env=prod#DbiResourceId
aws-lookup://rds/instance?team=payments&env=prod#Endpoint/Address
aws-lookup://sns/topic?team=payments#TopicArn
```

The function then makes a second call, Cloud Control `GetResource`, for the
matching resource and uses the value of that field.

- Fields are CloudFormation property names, as listed in the
  [CloudFormation resource reference][cfn-reference]: `DbiResourceId`, not
  `resource_id` or `resourceId`.
- Separate nested fields with `/`, and use a number for a list item:
  `Endpoint/Address`, `Tags/0/Value`. Escape a `/` in a property name as `~1`
  and a `~` as `~0`, as in JSON Pointer.
- The value must be a non-empty string or a number.
- If the field doesn't exist, the pipeline stops and the error lists the
  fields that do.
- The `#` must come after the tags. In `aws-lookup://s3/bucket#Field?a=b` the
  tags are part of the fragment, and the function rejects the URI because it
  has no tags.

## Input

All fields are optional.

```yaml
input:
  apiVersion: awslookup.fn.gideonmaree.dev/v1beta1
  kind: Input
  # Region to search when a resource has no spec.forProvider.region.
  region: eu-west-1
  # Name of the step credentials to use. Defaults to aws-creds.
  credentialsName: aws-creds
  # IAM role to assume before the lookup, for example in another account.
  assumeRoleARN: arn:aws:iam::123456789012:role/crossplane-lookup
  # Fail (default) or Create.
  onNotFound: Fail
```

The region comes from the resource's `spec.forProvider.region`, or from the
input's `region` when the resource has none.

## Credentials

The function uses the step credentials named by `credentialsName`. The secret
can have either:

- a `credentials` key with an AWS shared credentials file, the same format
  that provider-aws uses; or
- separate `aws_access_key_id`, `aws_secret_access_key` and optional
  `aws_session_token` keys.

```yaml
- step: adopt-existing-resources
  functionRef:
    name: function-aws-lookup
  credentials:
  - name: aws-creds
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: aws-creds
```

Without step credentials the function uses the default AWS credential chain,
so EKS Pod Identity or IRSA work through a `DeploymentRuntimeConfig`.

The identity needs:

- `tag:GetResources` for every lookup;
- `sts:AssumeRole` on the role, if you set `assumeRoleARN`;
- for lookups that read a field (`#<field>` and `rds/instance`):
  `cloudformation:GetResource`, plus the permissions of the CloudFormation
  read handler of the type. The handler runs with the function's identity.
  Some handlers need more than you might expect: `AWS::RDS::DBInstance` needs
  `rds:DescribeDBInstances` and several `ec2:Describe*` permissions, and
  `AWS::S3::Bucket` needs about twenty `s3:Get*` permissions. The schema of
  each type lists them under `handlers.read.permissions`
  (`aws cloudformation describe-type --type RESOURCE --type-name AWS::S3::Bucket`).
  The AWS managed policy `ReadOnlyAccess` covers them.

## Adopt safely

Once adopted, the provider manages the resource like any other: it changes
the resource to match `spec.forProvider`, and deletes it when the managed
resource is deleted. When you adopt resources from another tool:

- start with `managementPolicies: ["Observe"]` and compare the observed state
  with your composition before you allow changes;
- set `deletionPolicy: Orphan` until you are sure the composition is right;
- remove the resource from the old tool's state (`terraform state rm`, or a
  CloudFormation `DeletionPolicy: Retain` followed by removing it from the
  stack) so that two tools don't manage it.

## Development

```shell
# Run the unit tests
go test ./...

# Lint
golangci-lint run

# Regenerate the input CRD and deepcopy code after changing input/v1beta1
go generate ./...

# Build the runtime image and the Crossplane package
docker build . --tag=runtime
crossplane xpkg build -f package --embed-runtime-image=runtime
```

To try the function with `crossplane render`, see [example](example/).

[functions]: https://docs.crossplane.io/latest/composition/compositions/
[tagging-api]: https://docs.aws.amazon.com/resourcegroupstagging/latest/APIReference/API_GetResources.html
[cloud-control]: https://docs.aws.amazon.com/cloudcontrolapi/latest/APIReference/API_GetResource.html
[cfn-reference]: https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-template-resource-type-ref.html
[provider-aws]: https://github.com/crossplane-contrib/provider-upjet-aws
[provider-aws-externalname]: https://github.com/crossplane-contrib/provider-upjet-aws/blob/main/config/externalname.go
