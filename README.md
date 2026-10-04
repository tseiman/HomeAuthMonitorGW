# HomeAuthMonitorGW

HomeAuthMonitorGW is a small, read-only Linux monitoring gateway. It polls configured NUT and SNMPv3 sources, keeps the latest values in memory, and exposes them through an authenticated HTTPS/JSON API for systems such as Zabbix.

- HTTP requests never trigger device access.
- SNMP uses configured GET/BulkWalk operations only; there is no SET path.
- NUT uses `LIST VAR` only.
- Clients cannot supply device addresses, OIDs, or protocol commands.
- One failed source does not stop other collectors; its last successful snapshot becomes stale.

## Software dependencies

**Gateway runtime**

- Linux with systemd and standard Debian-family administration tools.
- No Go toolchain, Net-SNMP package, database, or CGO runtime is required.
- NUT (`nut-server`; `nut-client` provides `upsc`) is required only when this host reads a locally attached UPS.
- A TLS certificate and matching private key must be created or obtained separately, stored securely, and referenced in the configuration.

**Build and installation**

- Git.
- Go 1.23 or newer.
- `sudo` only when invoked by a normal user; root-only systems do not need it. Required system tools are checked automatically by `scripts/install.sh`.
- Internet access during `go mod download`, unless the Go module cache is already populated.

**Optional tools**

- `snmptranslate` from the Debian `snmp` package: required only on the workstation that runs `mib2json`; it is not a gateway runtime dependency.
- Node.js: optional, used only for additional Zabbix JavaScript tests.

Go dependencies are pinned through `go.mod` and `go.sum`: GoSNMP for SNMPv3 and `yaml.v3` for strict YAML parsing.

## Build and installation

The installer handles first installation and updates, accepts a locally modified checkout, and works directly as root. Its fast default downloads modules and builds; request the slower test/vet gates explicitly with `--verify`.

```bash
sudo apt update
sudo apt install --yes ca-certificates git golang-go

git clone https://github.com/tseiman/HomeAuthMonitorGW.git
cd HomeAuthMonitorGW
go version
./scripts/install.sh
```

The installer checks required commands and Go 1.23+, downloads checksummed modules, builds a static binary, and installs:

- `/usr/local/sbin/automation-gateway`
- `/etc/systemd/system/automation-gateway.service`
- `/etc/automation-gateway/config.yaml`
- `/etc/automation-gateway/mib-metadata.json`
- `/etc/automation-gateway/secrets/`
- `/etc/automation-gateway/tokens/`

The example configuration is intentionally not started. Configure the site-specific files, validate them, then start the service:

```bash
./scripts/install.sh --start
sudo systemctl status automation-gateway
sudo journalctl -u automation-gateway -n 50 --no-pager
```

Useful installer options:

- `--binary=PATH`: install a prebuilt binary; Go is not required on the target.
- `--verify`: run tests and vet before building.
- `--verbose` or `-v`: show detailed Go download, verification, and compilation progress.
- `--start`: explicitly enable and start an inactive valid installation.

Existing configuration, metadata, tokens, SNMP/NUT secrets, NUT configuration, and TLS files are preserved. Binary and unit files are replaced only when their content changes. Adjacent `.previous` copies are retained before replacement. Rollback is always an operator decision; the installer never restores them automatically.

### TLS and token permissions

The installer preserves these site-owned files during installation and updates. Directories must be `root:automation-gateway` with mode `0750`; token, certificate, and private-key files must be `root:automation-gateway` with mode `0640`. The service may read credentials but must not replace them.

Create or repair the directories:

```bash
sudo install -d -o root -g automation-gateway -m 0750 \
  /etc/automation-gateway/tokens \
  /etc/automation-gateway/tls
```

Install an existing certificate and key:

```bash
sudo install -o root -g automation-gateway -m 0640 \
  /path/to/fullchain.pem /etc/automation-gateway/tls/server.crt
sudo install -o root -g automation-gateway -m 0640 \
  /path/to/private-key.pem /etc/automation-gateway/tls/server.key
```

Generate and install a URL-safe bearer token without printing it:

```bash
openssl rand -base64 32 | tr -d '=\n' | tr '/+' '_-' | \
  sudo install -o root -g automation-gateway -m 0640 \
  /dev/stdin /etc/automation-gateway/tokens/zabbix
```

Repair existing files and verify everything:

```bash
sudo chown root:automation-gateway \
  /etc/automation-gateway/tokens/zabbix \
  /etc/automation-gateway/tls/server.crt \
  /etc/automation-gateway/tls/server.key
sudo chmod 0640 \
  /etc/automation-gateway/tokens/zabbix \
  /etc/automation-gateway/tls/server.crt \
  /etc/automation-gateway/tls/server.key

stat -c '%U:%G:%a %n' \
  /etc/automation-gateway/tokens \
  /etc/automation-gateway/tokens/zabbix \
  /etc/automation-gateway/tls \
  /etc/automation-gateway/tls/server.crt \
  /etc/automation-gateway/tls/server.key
```

Expected modes are `750` for both directories and `640` for all three files. Reference the token path under `authentication.bearer_token_files` and the certificate paths under `server.certificate` and `server.private_key`.

For a manual build:

```bash
go mod download
GOPROXY=off go test ./...
GOPROXY=off go vet ./...
CGO_ENABLED=0 go build -trimpath -o automation-gateway ./cmd/gateway
./automation-gateway --version
./scripts/install.sh --binary=./automation-gateway
```

## Cross-build and installation

Select the target from its **userspace architecture**, for example with `dpkg --print-architecture`:

- `armhf` → `GOARCH=arm GOARM=7`
- `i386` → `GOARCH=386`
- `arm64` → `GOARCH=arm64`
- `amd64` → `GOARCH=amd64`

Build on the build host:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -o automation-gateway-linux-armv7 ./cmd/gateway
CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -trimpath -o automation-gateway-linux-386 ./cmd/gateway
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o automation-gateway-linux-arm64 ./cmd/gateway
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o automation-gateway-linux-amd64 ./cmd/gateway
```

Copy the matching artifact and a repository checkout to the target, then run:

```bash
cd HomeAuthMonitorGW
./scripts/install.sh --binary=/path/to/automation-gateway-linux-armv7
```

The binary-install path still checks the target's systemd and administration commands, but it does not require Go or download modules.

## Configuration

Start with [`configs/config.example.yaml`](configs/config.example.yaml). Important settings:

- `server.listen`: HTTPS listen address.
- `server.certificate`, `server.private_key`: securely stored TLS files; the installer never creates or changes them.
- `authentication.bearer_token_files`: protected token files containing 16–4096 non-whitespace bytes.
- `authentication.allowed_clients`: exact client IPs or canonical CIDRs; forwarding headers are ignored.
- `sources[].driver`: `nut` or `snmp`.
- `sources[].poll_interval`, `stale_after`, `timeout`: bounded source timing.
- `sources[].nut.ups`: exact upsd UPS name.
- `sources[].snmp.oids`: fixed normal-poll OID allowlist.
- `sources[].snmp.discovery.root_oids`: optional fixed startup/reload BulkWalk roots.
- `sources[].snmp.metadata_file`: optional JSON generated by `mib2json`.
- CLI layering: repeat `--config=PATH` (short: `-C=PATH`) up to 32 times; later mappings override earlier keys, while lists and scalar values replace them. `--check`/`-c`, `--foreground`/`-f`, `--log-level=debug`/`-l=debug`, and `--version`/`-V` are accepted. CLI logging flags override YAML.

Site-owned configuration and secret files should normally be root-owned, group-readable by `automation-gateway`, and mode `0640`. Secret files must be regular files, not symlinks or devices, and must not be accessible to “other”. Configuration, secrets, tokens, and TLS directories must be `root:automation-gateway` with mode `0750`; the service account may read but must not replace its credentials.

If an older/manual setup made the token directory service-owned, repair it with:

```bash
sudo chown root:automation-gateway /etc/automation-gateway/tokens
sudo chmod 0750 /etc/automation-gateway/tokens
sudo chown root:automation-gateway /etc/automation-gateway/tokens/zabbix
sudo chmod 0640 /etc/automation-gateway/tokens/zabbix
```

Install an existing certificate and private key at the example configuration paths:

```bash
sudo install -d -o root -g automation-gateway -m 0750 /etc/automation-gateway/tls
sudo install -o root -g automation-gateway -m 0640 /path/to/fullchain.pem /etc/automation-gateway/tls/server.crt
sudo install -o root -g automation-gateway -m 0640 /path/to/private-key.pem /etc/automation-gateway/tls/server.key
```

Generate and install a URL-safe bearer token without printing it to the terminal:

```bash
openssl rand -base64 32 | tr -d '=\n' | tr '/+' '_-' | \
  sudo install -o root -g automation-gateway -m 0640 /dev/stdin /etc/automation-gateway/tokens/zabbix
```

Reference `/etc/automation-gateway/tokens/zabbix` under `authentication.bearer_token_files`.

Validate with the real service identity:

```bash
sudo -u automation-gateway /usr/local/sbin/automation-gateway \
  --config=/etc/automation-gateway/config.yaml \
  --check
```

Reload after reloadable changes; restart after changing the listen address or server timeout limits:

```bash
# Choose reload for reloadable source/auth/TLS changes:
sudo systemctl reload automation-gateway
# Or restart after changing listen/timeout settings:
sudo systemctl restart automation-gateway
sudo journalctl -u automation-gateway -n 50 --no-pager
```

At `info`, the gateway logs every HTTP request and every successful poll; poll failures are always errors. At `debug`, it additionally logs poll starts and returned metric values. Follow service activity with:

```bash
sudo journalctl -fu automation-gateway
/usr/local/sbin/automation-gateway --foreground --log-level=debug
```

### NUT

- The gateway does not access USB directly: UPS → NUT driver → local upsd → gateway.
- Keep upsd on `127.0.0.1:3493` unless the site explicitly requires otherwise.
- Use the exact UPS section name from `/etc/nut/ups.conf` in `sources[].nut.ups`.
- Test locally with `upsc UPS_NAME@127.0.0.1` before starting the gateway.
- `upsd` listening on port 3493 is not sufficient: the matching `nut-driver@UPS_NAME` must be active and `upsc` must return values. Otherwise health is `degraded`, the source is `stale`, and the gateway error log contains the concrete upsd or connection error.

### SNMPv3

- Configure the device address, authPriv account, auth protocol, privacy protocol, protected passphrase files containing 8–255 bytes, and a fixed OID list.
- Normal polling partitions the fixed list into bounded GET requests. `max_oids_per_request` defaults to `16` and accepts `1`–`32`; lower it for constrained embedded agents without removing OIDs from the allowlist. The WAGO 750-880 was observed to reject a 19-varbind GET with an unrecognized SNMPv3 report PDU while accepting 18, so `16` leaves a safety margin.
- A failed GET log identifies the affected one-based OID range, for example `OIDs 17-32 of 132`.
- Keep each OID only once. Duplicate entries waste device request capacity.
- Keep legacy SHA1/DES devices isolated inside the automation network.
- Do not treat a username such as `readonly` as proof that the device rejects writes; gateway safety comes from network policy and the absence of write code.

## MIB metadata with `mib2json`

A MIB describes names, numeric OIDs, units, types, and enum values. It does not tell the gateway what to poll: selected numeric OIDs must still be placed in `sources[].snmp.oids` or under a reviewed discovery root.

Install and build the offline tool on an admin/build workstation:

```bash
sudo apt install --yes snmp
snmptranslate -V
go build -trimpath -o mib2json ./cmd/mib2json
```

### Which MIB files are needed?

1. Obtain the MIB package for the exact device family and firmware from the device manufacturer or its support portal.
2. The primary file declares its module name near `DEFINITIONS ::= BEGIN`.
3. Its `IMPORTS` section references other modules after `FROM`. These are **dependent MIBs**. Dependencies can import further modules recursively.
4. Put the complete vendor package and required standard/vendor dependencies into one or more local directories. Do not rename module declarations inside the files.
5. Run `--list-symbols`. Messages such as `Cannot find module (SNMPv2-SMI)` name the missing dependency. Obtain that module from the vendor package or a trusted distribution source, add its directory, and repeat until no module/import errors remain.

Debian's `snmp` package provides `snmptranslate` but may not include all IETF MIB files. A vendor package may therefore need additional standard MIBs even though the command itself is installed.

List available symbols and numeric OIDs:

```bash
./mib2json \
  --mib-dir ./mibs/vendor \
  --mib-dir ./mibs/dependencies \
  --module VENDOR-MIB \
  --list-symbols
```

The list includes structural nodes and symbols imported from dependencies. Use the device manual/OID list and the primary MIB to select the readable `OBJECT-TYPE` entries that represent the values you actually need.

Convert only reviewed objects:

```bash
./mib2json \
  --mib-dir ./mibs/vendor \
  --mib-dir ./mibs/dependencies \
  --module VENDOR-MIB \
  --object deviceTemperature \
  --object deviceAlarmState \
  --output ./vendor-metadata.json

sudo install -o root -g automation-gateway -m 0640 \
  ./vendor-metadata.json \
  /etc/automation-gateway/vendor-metadata.json
```

Then set `sources[].snmp.metadata_file`, add the required numeric OIDs to the configured allowlist, and run the configuration check. `mib2json` never contacts a device and accepts no address or credentials. Review MIB licensing before redistributing vendor files or generated descriptions.

## API and Zabbix

Every endpoint requires both an allowed TCP peer and a bearer token:

- `GET /api/v1/health`
- `GET /api/v1/sources`
- `GET /api/v1/sources/{name}`
- `GET /api/v1/sources/{name}/metrics`
- `GET /api/v1/sources/{name}/discovery`
- `GET /api/v1/metrics`

Import [`zabbix/template_homeauthmonitorgw.yaml`](zabbix/template_homeauthmonitorgw.yaml), set `{$AUTOMATION_GATEWAY_URL}`, secret `{$AUTOMATION_GATEWAY_TOKEN}`, `{$AUTOMATION_GATEWAY_INTERVAL}`, and `{$AUTOMATION_GATEWAY_TIMEOUT}`, and keep certificate verification enabled. Add the Zabbix server/proxy's actual TCP source address to `authentication.allowed_clients`. The template uses one HTTP master item and dependent discovery/items; it never contacts NUT or SNMP devices directly.

## Update and rollback

Update from a clean checkout with the same installer:

```bash
cd HomeAuthMonitorGW
git status --short
git pull --ff-only
./scripts/install.sh
sudo systemctl status automation-gateway
```

If the operator chooses to restore existing `.previous` files:

```bash
sudo install -o root -g root -m 0755 /usr/local/sbin/automation-gateway.previous /usr/local/sbin/automation-gateway
sudo install -o root -g root -m 0644 /etc/systemd/system/automation-gateway.service.previous /etc/systemd/system/automation-gateway.service
sudo systemctl daemon-reload
sudo systemctl restart automation-gateway
```

## Security summary

- Keep forwarding disabled and permit HTTPS only from approved monitoring clients.
- Keep upsd loopback-only and field-device protocols inside the automation network.
- Never place tokens, SNMP passphrases, or private keys in YAML, source control, command lines, tickets, or logs.
- Keep every API endpoint, including health, behind the client allowlist and bearer-token policy.
- Discovery roots and normal OIDs are administrator configuration, never API input.

## License

See [LICENSE](LICENSE).
