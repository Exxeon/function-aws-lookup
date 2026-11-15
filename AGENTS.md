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
| `internal/lookup/` | URI parsing (`uri.go`), supported resource types (`types.go`), Cloud Control field extraction (`field.go`). No AWS calls. |
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
- Make at most one Tagging API call per region and query, and one Cloud Control call per
  resource, in a request.

## Add a resource type

Each entry in `internal/lookup/types.go` must be checked against two sources. Don't guess.

1. The external name in provider-upjet-aws,
   [`config/externalname.go`](https://github.com/crossplane-contrib/provider-upjet-aws/blob/main/config/externalname.go).
   `IdentifierFromProvider` means the Terraform ID, which may not be in the ARN; check the
   Terraform resource's import docs. If the external name isn't the ARN or a part of it, set
   `DefaultField` to the Cloud Control property that holds it (see `rds/instance`).
2. The CloudFormation schema: `primaryIdentifier` decides `CloudControlUsesARN`, and the type
   needs a `read` handler. Get the schemas with
   `aws cloudformation describe-type --type RESOURCE --type-name AWS::...` or from
   `https://schema.cloudformation.<region>.amazonaws.com/CloudformationSchema.zip`.

Then add entries to the tables in `internal/lookup/types_test.go` and the supported types table in
`README.md`.

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
go generate ./...                  # after changing input/v1beta1: regenerates deepcopy and package/input/
go mod tidy                        # CI fails if go.mod or go.sum change

go run . --insecure --debug        # run locally, then in example/:
crossplane render xr.yaml composition.yaml functions.yaml --function-credentials=secret.yaml
```

`crossplane render` needs Docker for the other functions in `example/functions.yaml`.

## Git and CI

- Don't push to `main`. Work on a branch and open a pull request.
- Commit messages follow Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`).
- `.github/workflows/ci.yml` lints, tests and builds the package for amd64 and arm64 on every pull
  request. On pushes to `main` it also pushes the package to `ghcr.io/exxeon/function-aws-lookup`
  (lowercase; registry names can't contain capitals). A `workflow_dispatch` run with a `version`
  input publishes a release version.

## Writing

- British spelling in `README.md` and comments (organisation, behaviour).
- This is a public Exxeon project. Don't mention employers, clients, internal systems or private
  infrastructure in code, docs, examples or commit messages.
