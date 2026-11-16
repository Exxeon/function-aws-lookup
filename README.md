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
    crossplane.io/external-name: aws-lookup://s3?team=payments&environment=prod
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
   the annotation: the ID from its ARN, or, if the URI names a field, that
   field of the resource as read with the [Cloud Control API][cloud-control].
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
aws-lookup://<service>[/<type>]?<tag>=<value>&<tag>=<value>[#<field>]
```

`<service>` and `<type>` are the service and resource type in the ARN of the
resource you're looking for, as the [Tagging API][tagging-api] uses them:

| Resource | ARN | URI |
| --- | --- | --- |
| VPC | `arn:aws:ec2:eu-west-1:123456789012:vpc/vpc-0abc` | `aws-lookup://ec2/vpc?...` |
| RDS instance | `arn:aws:rds:eu-west-1:123456789012:db:orders` | `aws-lookup://rds/db?...` |
| EFS file system | `arn:aws:elasticfilesystem:eu-west-1:123456789012:file-system/fs-0abc` | `aws-lookup://elasticfilesystem/file-system?...` |
| Log group | `arn:aws:logs:eu-west-1:123456789012:log-group:/aws/app` | `aws-lookup://logs/log-group?...` |
| REST API | `arn:aws:apigateway:eu-west-1::/restapis/a1b2c3` | `aws-lookup://apigateway/restapis?...` |
| S3 bucket | `arn:aws:s3:::my-bucket` | `aws-lookup://s3?...` |
| SNS topic | `arn:aws:sns:eu-west-1:123456789012:alerts` | `aws-lookup://sns?...` |

Leave out the type for resources whose ARN has none, such as S3 buckets, SNS
topics and SQS queues. Without a type, the function ignores other resources of
the service, such as S3 access points. Any service and type that the Tagging
API supports works; the function has no list of supported types.

- All tags must match.
- Repeat a tag to accept several values: `env=prod&env=production`.
- Leave the value out to match any value: `legacy` or `legacy=`.
- URL-encode special characters: `aws%3Acloudformation%3Astack-name=orders`.
- The optional `#<field>` goes at the end, after the tags. See
  [Choose the field](#choose-the-field).

## The external name

Without a field, the external name is the part of the ARN after the resource
type:

| ARN | External name |
| --- | --- |
| `arn:aws:ec2:eu-west-1:123456789012:vpc/vpc-0abc` | `vpc-0abc` |
| `arn:aws:rds:eu-west-1:123456789012:db:orders` | `orders` |
| `arn:aws:ecr:eu-west-1:123456789012:repository/team/app` | `team/app` |
| `arn:aws:s3:::my-bucket` | `my-bucket` |

That is what [provider-upjet-aws][provider-aws] expects for most types, but
not all. Check the type's entry in its
[`config/externalname.go`][provider-aws-externalname]. When the provider
expects something else, such as the full ARN or an ID that isn't in the ARN,
name the field that holds it.

## Choose the field

```
aws-lookup://rds/db?team=payments&env=prod#DbiResourceId
aws-lookup://rds/db?team=payments&env=prod#Endpoint/Address
aws-lookup://secretsmanager/secret?team=payments#Id
```

RDS instances are a common case: since Terraform AWS provider v5 their
external name in provider-upjet-aws is the DBI resource ID (`db-ABC123...`),
which is not in the ARN, so use `#DbiResourceId`.

With a field, the function reads the matching resource with Cloud Control
`GetResource` and uses the value of that field.

- Fields are CloudFormation property names, as listed in the
  [CloudFormation resource reference][service-reference]: https://docs.aws.amazon.com/service-authorization/latest/reference/service-reference.html
[cfn-reference]: `DbiResourceId`, not
  `resource_id` or `resourceId`.
- Separate nested fields with `/`, and use a number for a list item:
  `Endpoint/Address`, `Tags/0/Value`. Escape a `/` in a property name as `~1`
  and a `~` as `~0`, as in JSON Pointer.
- The value must be a non-empty string or a number.
- If the field doesn't exist, the pipeline stops and the error lists the
  fields that do.
- The `#` must come after the tags. In `aws-lookup://s3#Field?a=b` the tags
  are part of the fragment, and the function rejects the URI because it has
  no tags.

Cloud Control needs the resource's CloudFormation type, which the ARN doesn't
contain. The function finds it in a table that maps the ARN formats of all
taggable resources to CloudFormation types. AWS doesn't publish that mapping,
so `internal/lookup/gen` derives it from AWS's CloudFormation resource schemas
and the [Service Authorization Reference][service-reference]. It covers
almost all taggable CloudFormation types that Cloud Control can read; the
rest have no IAM resource type of their own.

Some CloudFormation types share an ARN format, such as the DataSync location
types or the classic and current load balancers. The function then tries each
of them, with the ID and with the ARN as the Cloud Control identifier, until
one exists.

## Input

All fields are optional.

```yaml
input:
  apiVersion: awslookup.fn.exxeon.io/v1beta1
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
- for lookups that read a field (`#<field>`):
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

## Publishing

CI publishes a multi-platform Crossplane package for `linux/amd64` and
`linux/arm64` to `ghcr.io/exxeon/function-aws-lookup` after lint, unit tests
and both builds pass. Pull requests build and check the package without
publishing it.

- Pushes to `main` or a `release-*` branch publish a development version:
  `v0.0.0-<UTC-commit-timestamp>-<12-character-commit-SHA>`.
- Pushing a version tag such as `v0.1.0` or `v0.1.0-rc.1` publishes that
  version. Tag the merged commit you want to release:

  ```shell
  git tag v0.1.0 <commit>
  git push origin v0.1.0
  ```

- To publish manually, run the **CI** workflow in GitHub Actions, select
  the branch or tag, and supply `version`. Leave it empty for a development
  version on a branch, or to use the selected tag's version.

Release versions must use `vMAJOR.MINOR.PATCH`, optionally followed by a
prerelease suffix such as `-rc.1`. Build metadata (`+...`) is not supported
in registry tags. Publishing a package does not create a GitHub Release.
The workflow's run summary shows the published package reference.

Publishing uses the repository's `GITHUB_TOKEN` with `packages: write`; no
separate registry secret is needed. For public installation, set the GHCR
package's visibility to **Public** in its GitHub package settings after its
first publication.

Install a published version with:

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Function
metadata:
  name: function-aws-lookup
spec:
  package: ghcr.io/exxeon/function-aws-lookup:v0.1.0
```

[functions]: https://docs.crossplane.io/latest/composition/compositions/
[tagging-api]: https://docs.aws.amazon.com/resourcegroupstagging/latest/APIReference/API_GetResources.html
[cloud-control]: https://docs.aws.amazon.com/cloudcontrolapi/latest/APIReference/API_GetResource.html
[service-reference]: https://docs.aws.amazon.com/service-authorization/latest/reference/service-reference.html
[cfn-reference]: https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-template-resource-type-ref.html
[provider-aws]: https://github.com/crossplane-contrib/provider-upjet-aws
[provider-aws-externalname]: https://github.com/crossplane-contrib/provider-upjet-aws/blob/main/config/externalname.go
