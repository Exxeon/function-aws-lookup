# function-aws-lookup

A Crossplane composition function, written in Go, that adopts existing AWS resources by tag. It
replaces `aws-lookup://` URIs in the `crossplane.io/external-name` annotation of desired composed
resources with the external name of the one AWS resource whose tags match. `README.md` describes
the behaviour for users; read it before changing behaviour.

Public repository: https://github.com/Exxeon/function-aws-lookup. Go module:
`github.com/Exxeon/function-aws-lookup`. Input API group: `awslookup.fn.exxeon.io`.

## Layout

| Path | Contents |
| --- | --- |
| `main.go` | CLI and gRPC server. Wires `awsclient.NewFinder` into the Function. |
| `fn.go` | `RunFunction`: walks desired resources, resolves URIs, writes results. |
| `input/v1beta1/` | Function input type. `zz_generated.deepcopy.go` is generated. |
| `internal/lookup/` | URI parsing (`uri.go`), ARN parsing (`arn.go`), ARN-to-CloudFormation lookup (`cloudformation.go`), Cloud Control field extraction (`field.go`). No AWS calls. |
| `internal/lookup/gen/` | Generator for `internal/lookup/zz_generated_cloudformation.go`. |
| `internal/awsclient/` | AWS SDK code: Tagging API search, Cloud Control `GetResource`, credentials. |
| `package/` | Crossplane package metadata and the generated input CRD. |
| `example/` | Manifests for `crossplane render`. |

## Invariants

Keep these when changing `fn.go`:

- Never send an `aws-lookup://` URI on to the provider. If a URI can't be resolved (bad URI, no
  region, AWS error, several matches, missing field, no match with `onNotFound: Fail`), return a
  fatal result. Crossplane then applies nothing, which is safe.
- If the observed resource already has an external name that isn't a URI, copy it into the
  desired resource and don't call AWS. Adoption happens once; an external name never changes
  afterwards.
- Don't create AWS clients or call AWS for requests that have no URIs to resolve.
- Make at most one Tagging API call per region and query in a request, and read each resource
  with Cloud Control at most once.
- Don't add per-type rules. The URI names the service and type as they appear in the ARN, and the
  external name is the ARN's ID unless the URI names a field. The only per-type data is the
  generated ARN-to-CloudFormation table.

## The CloudFormation table

`internal/lookup/zz_generated_cloudformation.go` maps the ARN formats of taggable resources to
CloudFormation types, so that `#field` lookups know which type to read with Cloud Control. Never
edit it by hand. Regenerate it when AWS adds services:

```sh
cd internal/lookup && go run ./gen
```

The generator downloads AWS's CloudFormation resource schemas and the Service Authorization
Reference, and matches each taggable CloudFormation type to the IAM resource type its handlers act
on. If a mapping is wrong, fix the matching rules in `gen/main.go`, not the output, and add the
ARN to the table in `cloudformation_test.go`. Check the diff of the generated file before
committing it.

## Tests

- Write unit tests with Ginkgo and Gomega (`Describe`, `It`, `DescribeTable`), never plain
  `func TestX(t *testing.T)`. Each package has a `*_suite_test.go`.
- Unit tests never call AWS. `fn_test.go` uses `fakeFinder`, which implements `awsclient.Finder`.
- Use made-up account IDs (`123456789012`) and names in tests and examples.

## Commands

```sh
go test ./...                      # unit tests
golangci-lint fmt ./...            # format (gci, gofumpt)
golangci-lint run                  # lint; CI runs the same config
go generate ./...                  # regenerates deepcopy, package/input/ and the CloudFormation table (needs internet)
go mod tidy                        # CI fails if go.mod or go.sum change

go run . --insecure --debug        # run locally, then in example/:
crossplane render xr.yaml composition.yaml functions.yaml --function-credentials=secret.yaml
```

`crossplane render` needs Docker for the other functions in `example/functions.yaml`.

## Git and CI

- Don't push to `main`. Work on a branch and open a pull request. The "Protect main" ruleset
  enforces this for everyone, admins included:
  - a maintainer from `.github/CODEOWNERS` other than the last pusher must approve, and new pushes
    dismiss earlier approvals;
  - `lint`, `unit-test`, `build (amd64)` and `build (arm64)` must pass on a branch that is up to
    date with `main`;
  - all review threads must be resolved;
  - pull requests are squash-merged, with the pull request title as the commit message, so the
    title must follow Conventional Commits.
- Commit messages follow Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`).
- `.github/workflows/ci.yml` lints, tests and builds the package for amd64 and arm64 on every pull
  request. On pushes to `main` it also pushes the package to `ghcr.io/exxeon/function-aws-lookup`
  (lowercase; registry names can't contain capitals), after lint, tests and both builds pass.
  Pushing a `v*` version tag or a `workflow_dispatch` run with a `version` input publishes a
  release version. Release versions use `vMAJOR.MINOR.PATCH` with an optional prerelease suffix.

## Writing

- British spelling in `README.md` and comments (organisation, behaviour).
- This is a public Exxeon project. Don't mention employers, clients, internal systems or private
  infrastructure in code, docs, examples or commit messages.
