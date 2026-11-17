# Contributing

Thank you for helping to improve function-aws-lookup. Bug reports, feature
requests and pull requests are welcome.

## Before you start

For anything larger than a small fix, open an issue first so that we can agree
on the approach. The README describes how the function behaves; a change to that
behaviour needs a README update in the same pull request.

## Developer Certificate of Origin

By contributing you agree to the [Developer Certificate of Origin](DCO) (DCO):
you confirm that you have the right to submit the change under the project's
[Apache 2.0 licence](LICENSE).

Sign off every commit with `git commit -s`. This adds a trailer with the name
and email of the commit author:

```
Signed-off-by: Jane Doe <jane@example.com>
```

The `dco` check fails a pull request if any of its commits lacks a sign-off
that matches the commit's author email. To sign off commits you already made:

```shell
git rebase --signoff origin/main
git push --force-with-lease
```

## Pull requests

- Keep each pull request to one change, with tests.
- Write unit tests with [Ginkgo](https://onsi.github.io/ginkgo/) and
  [Gomega](https://onsi.github.io/gomega/). Unit tests never call AWS; use the
  fake finder in `fn_test.go`.
- Run the checks that CI runs:

  ```shell
  go test ./...
  golangci-lint fmt ./...
  golangci-lint run
  go mod tidy
  ```

  After changing `input/v1beta1`, or to pick up new AWS resource types, run
  `go generate ./...` and commit the result. It needs internet access.
- The pull request title must follow
  [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/), for
  example `fix: handle empty tag values`. Pull requests are squash-merged with
  the title as the commit message, and the title decides the next release: see
  [Publishing](README.md#publishing). Mark breaking changes with `!`, as in
  `feat!: rename the input field`.
- A maintainer other than the last person to push must approve the pull
  request, all checks must pass, and all review threads must be resolved
  before it can be merged.

## Reporting security issues

Don't open a public issue for a security problem. See [SECURITY.md](SECURITY.md).

## Code of conduct

This project follows the
[CNCF Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md).
