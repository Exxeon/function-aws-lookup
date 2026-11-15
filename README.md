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
   the annotation.
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
aws-lookup://<service>/<type>?<tag>=<value>&<tag>=<value>
```

- All tags must match.
- Repeat a tag to accept several values: `env=prod&env=production`.
- Leave the value out to match any value: `legacy` or `legacy=`.
- URL-encode special characters: `aws%3Acloudformation%3Astack-name=orders`.

Supported types, with the external names that
[provider-upjet-aws][provider-aws] expects:

| URI | External name |
| --- | --- |
| `dynamodb/table` | table name |
| `ec2/instance` | instance ID |
| `ec2/internet-gateway` | internet gateway ID |
| `ec2/nat-gateway` | NAT gateway ID |
| `ec2/route-table` | route table ID |
| `ec2/security-group` | security group ID |
| `ec2/subnet` | subnet ID |
| `ec2/vpc` | VPC ID |
| `ecr/repository` | repository name |
| `efs/file-system` | file system ID |
| `eks/cluster` | cluster name |
| `elasticache/replication-group` | replication group ID |
| `elb/load-balancer` | ARN (application, network and gateway load balancers) |
| `elb/target-group` | ARN |
| `kms/key` | key ID |
| `lambda/function` | function name |
| `logs/log-group` | log group name |
| `rds/cluster` | cluster identifier |
| `s3/bucket` | bucket name |
| `secretsmanager/secret` | ARN |
| `sns/topic` | topic name |

To add a type, check its external name configuration in
[provider-upjet-aws][provider-aws-externalname] and add it to
`internal/lookup/types.go`. Types that the Tagging API doesn't support, such as
IAM roles, can't be looked up.

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

The identity needs `tag:GetResources`, plus `sts:AssumeRole` on the role if you
set `assumeRoleARN`.

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
[provider-aws]: https://github.com/crossplane-contrib/provider-upjet-aws
[provider-aws-externalname]: https://github.com/crossplane-contrib/provider-upjet-aws/blob/main/config/externalname.go
