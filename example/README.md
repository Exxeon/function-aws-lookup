# Examples

Run the function locally and call it with `crossplane render`. `render` needs
Docker for function-go-templating.

```shell
# Run the function locally
go run . --insecure --debug
```

The bucket below was already adopted, so the function keeps its external name
and doesn't call AWS:

```shell
crossplane render xr.yaml composition.yaml functions.yaml \
  --observed-resources=observed.yaml \
  --function-credentials=secret.yaml
```

To look up a real bucket, put real credentials in `secret.yaml`, change the
tags in `xr.yaml` to match a bucket in `eu-west-1`, and leave out
`--observed-resources`:

```shell
crossplane render xr.yaml composition.yaml functions.yaml \
  --function-credentials=secret.yaml
```
