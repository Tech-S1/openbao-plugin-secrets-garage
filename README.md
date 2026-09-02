# OpenBao Garage Secrets Plugin

OpenBao secrets engine that issues short-lived Garage S3 access keys via the Garage Admin API.

## Install

Download a binary from [Releases](https://github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/releases) (`linux`/`darwin`, `amd64`/`arm64`) and verify with `SHA256SUMS`.

Copy the binary into your OpenBao `plugin_directory`.

```bash
PLUGIN=openbao-plugin-secrets-garage
PLUGIN_DIR="${OPENBAO_PLUGIN_DIR:-/var/lib/openbao/plugins}"
OPENBAO_USER="${OPENBAO_USER:-openbao}"

chmod +x "${PLUGIN}_"*
sudo mv "${PLUGIN}_"* "$PLUGIN_DIR/$PLUGIN"
sudo chown "$OPENBAO_USER:$OPENBAO_USER" "$PLUGIN_DIR/$PLUGIN"
sudo chmod 0750 "$PLUGIN_DIR/$PLUGIN"
```

Register the plugin (replace the checksum with the value from `SHA256SUMS`):

```bash
bao plugin register -sha256="<sha256>" secret openbao-plugin-secrets-garage
bao secrets enable -path=garage openbao-plugin-secrets-garage
```



## Requirements

- OpenBao with external plugin support
- Garage cluster with the Admin API reachable from OpenBao
- Garage admin bearer token with permission to create and delete access keys



## Quick start

```bash
export BAO_ADDR=https://openbao.example.com
export BAO_TOKEN=...

bao write garage/config \
  address="http://127.0.0.1:3903" \
  token="$GARAGE_ADMIN_TOKEN" \
  default_ttl=300 \
  max_ttl=900

bao write garage/roles/myapp \
  bucket=my-bucket \
  ttl=5m \
  max_ttl=15m \
  read=true \
  write=true

bao read garage/creds/myapp
```

Returned credentials include `access_key_id`, `secret_access_key`, `expiration`, and `bucket`. The Garage key is deleted when the lease is revoked or expires. Renew extends the Garage key expiration to match the new lease.

## Config



### `garage/config`


| Key           | Purpose                                      |
| ------------- | -------------------------------------------- |
| `address`     | Garage Admin API base URL (required)         |
| `token`       | Garage Admin API bearer token (required)     |
| `default_ttl` | Default lease TTL in seconds (default `300`) |
| `max_ttl`     | Max lease TTL in seconds (default `900`)     |




### `garage/roles/:name`


| Key       | Purpose                                     |
| --------- | ------------------------------------------- |
| `bucket`  | Garage bucket global alias (required)       |
| `ttl`     | Lease TTL for keys from this role           |
| `max_ttl` | Max lease TTL for keys from this role       |
| `read`    | Grant read on the bucket (default `true`)   |
| `write`   | Grant write on the bucket (default `true`)  |
| `owner`   | Grant owner on the bucket (default `false`) |




### `garage/creds/:name`

Read (or update) to issue a new leased access key for the named role.

## Develop

Needs Go 1.26+.

```bash
make build
make test
```



### Pre-commit checks

Optional git hook runs the same lint/vuln/test checks as CI:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1
go install golang.org/x/vuln/cmd/govulncheck@v1.7.0
git config core.hooksPath .githooks
```

Or run manually: `make check` (`lint` + `vuln` + `test`).