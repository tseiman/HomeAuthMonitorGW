# HomeAuthMonitorGW

HomeAuthMonitorGW is a small, read-only monitoring gateway for a KUNBUS Revolution Pi Core 3. It polls local NUT/upsd and an automation-VLAN device over SNMPv3, keeps independent in-memory source snapshots, and exposes only a controlled HTTPS/JSON API for Zabbix.

The service is **not** an SNMP, NUT, or HTTP proxy. HTTP clients cannot choose an OID, protocol operation, NUT command, or device address. The production SNMP adapter implements GET and BulkWalk only; there is no SNMP SET path.

## Contents

- [Architecture and trust boundaries](#architecture-and-trust-boundaries)
- [Supported sources and constraints](#supported-sources-and-constraints)
- [Build and test](#build-and-test)
- [Blank-host installation](#blank-host-installation)
- [NUT, upsd, and USB](#nut-upsd-and-usb)
- [WAGO SNMPv3](#wago-snmpv3)
- [Configuration and secrets](#configuration-and-secrets)
- [MIB metadata and discovery](#mib-metadata-and-discovery)
- [TLS, LEGO, and reload](#tls-lego-and-reload)
- [API reference](#api-reference)
- [Zabbix 74 example](#zabbix-74-example)
- [Service operation and logging](#service-operation-and-logging)
- [Firewall migration](#firewall-migration)
- [Update, rollback, and removal](#update-rollback-and-removal)
- [Troubleshooting](#troubleshooting)
- [Security considerations](#security-considerations)

## Architecture and trust boundaries

```mermaid
flowchart LR
  subgraph AV[Automation VLAN - legacy protocol boundary]
    Q[Phoenix QUINT] -->|USB| N[NUT driver]
    N --> U[upsd 127.0.0.1:3493]
    U -->|LIST VAR only| G[HomeAuthMonitorGW]
    W[WAGO 750-880] -->|SNMPv3 authPriv SHA1/DES; read operations only| G
  end
  G --> C[Per-source RAM cache]
  C -->|HTTPS + bearer token| Z[Zabbix 7.4]
  L[LEGO / ACME] -->|certificate files; no gateway ACME code| G
  A[Operator] -->|SIGHUP through systemctl reload| G
```

Trust boundaries:

1. USB and SNMP are inside the automation network. Old SHA1/DES support remains there because the target requires it.
2. The RevPi is an endpoint, not a router. Do not enable IP forwarding.
3. Zabbix reaches one HTTPS listener. The API returns configured source data only.
4. Bearer, SNMP auth, and SNMP privacy values live in separate files. They are never returned or intentionally logged.
5. Each collector polls independently. HTTP never performs a poll. A failed collector retains its last-known-good values and marks them stale without stopping another source or the daemon.

The internal `drivers.Collector` and optional `drivers.Discoverer` interfaces use protocol-independent `metrics.Metric` and `metrics.Definition` values. Drivers are compiled into one binary. A future Unix-domain-socket adapter can implement the same boundary; dynamic plugins and IPC are intentionally outside v1.

## Supported sources and constraints

- **NUT:** TCP client to upsd, normally `127.0.0.1:3493`; sends only `LIST VAR <configured-ups>`.
- **SNMP:** SNMPv3 authPriv with SHA/SHA1 authentication and DES privacy through pinned GoSNMP. Normal polls issue GET for a fixed configuration allowlist. Optional startup/reload discovery issues BulkWalk for fixed roots.
- **HTTPS:** one TLS 1.2-or-newer server and source-oriented v1 API.

No database, generic protocol endpoint, write/control command, pprof listener, directory server, ACME client, Modbus replacement, or CODESYS replacement is included.

## Build and test

Requirements are Go 1.23 or newer and Git. Dependencies are deliberately limited:

- `github.com/gosnmp/gosnmp v1.42.1`: SNMPv3/authPriv and read operations.
- `gopkg.in/yaml.v3 v3.0.1`: strict YAML decoding.

```bash
git clone https://github.com/tseiman/HomeAuthMonitorGW.git
cd HomeAuthMonitorGW
go version
go mod download
go test ./...
go vet ./...
go build -trimpath -ldflags "-s -w -X main.version=1.0.0 -X main.commit=$(git rev-parse HEAD) -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o automation-gateway ./cmd/gateway
./automation-gateway --version
```

For a 32-bit Revolution Pi Core 3, cross-build either target explicitly:

```bash
GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -o automation-gateway-linux-armv7 ./cmd/gateway
GOOS=linux GOARCH=386 go build -trimpath -o automation-gateway-linux-386 ./cmd/gateway
```

`arm/7` is the usual Raspberry Pi 3/RevPi target. Confirm the installed OS with `dpkg --print-architecture` before selecting an artifact. Builds are pure Go and do not require CGO.

## Blank-host installation

The recommended path builds on a workstation or directly on a RevPi with Go 1.23+. On a blank Debian/Raspbian-style host:

```bash
sudo apt update
sudo apt install --yes ca-certificates git golang-go nut nut-client openssl curl

go version
git --version
```

Stop if `go version` is older than 1.23; install a supported Go toolchain from the official Go distribution or deploy the cross-built ARMv7 binary instead. Then build and stage:

```bash
git clone https://github.com/tseiman/HomeAuthMonitorGW.git
cd HomeAuthMonitorGW
go mod download
go test ./...
go build -trimpath -o automation-gateway ./cmd/gateway
sudo install -o root -g root -m 0755 automation-gateway /usr/local/sbin/automation-gateway
```

Create a non-login service identity and protected paths:

```bash
sudo adduser --system --group --no-create-home --home /nonexistent automation-gateway
sudo install -d -o root -g automation-gateway -m 0750 /etc/automation-gateway
sudo install -d -o root -g automation-gateway -m 0750 /etc/automation-gateway/secrets
sudo install -o root -g automation-gateway -m 0640 configs/config.example.yaml /etc/automation-gateway/config.yaml
sudo install -o root -g automation-gateway -m 0640 configs/mib-metadata.example.json /etc/automation-gateway/mib-metadata.json
sudo install -o root -g root -m 0644 systemd/automation-gateway.service /etc/systemd/system/automation-gateway.service
```

Provision secrets without putting them in shell arguments or history. Run each command, enter the value, and press Enter; every physical target must be provisioned separately:

```bash
sudo sh -c 'umask 027; read -r value; printf "%s\n" "$value" > /etc/automation-gateway/token; unset value'
sudo sh -c 'umask 027; read -r value; printf "%s\n" "$value" > /etc/automation-gateway/secrets/wago-auth; unset value'
sudo sh -c 'umask 027; read -r value; printf "%s\n" "$value" > /etc/automation-gateway/secrets/wago-priv; unset value'
sudo chown root:automation-gateway /etc/automation-gateway/token /etc/automation-gateway/secrets/wago-*
sudo chmod 0640 /etc/automation-gateway/token /etc/automation-gateway/secrets/wago-*
```

Configure NUT, SNMP, host addresses, and TLS as described below. Validate before starting:

```bash
sudo -u automation-gateway /usr/local/sbin/automation-gateway --config /etc/automation-gateway/config.yaml --check
sudo systemd-analyze verify /etc/systemd/system/automation-gateway.service
sudo systemctl daemon-reload
sudo systemctl enable --now automation-gateway
sudo systemctl status automation-gateway
sudo journalctl -u automation-gateway -n 50 --no-pager
```

The unit grants only `CAP_NET_BIND_SERVICE`, so the unprivileged process can bind port 443. Port 8443 is an alternative: change `server.listen` and remove both capability directives from the unit.

## NUT, upsd, and USB

The gateway does not access USB itself:

```text
Phoenix QUINT -> USB -> NUT hardware driver -> local upsd -> gateway LIST VAR
```

Identify the UPS as an administrator:

```bash
lsusb
sudo nut-scanner -U
```

Use the NUT driver reported for the device and follow the installed driver's man page. A typical `/etc/nut/ups.conf` shape is shown below; the actual driver and USB selectors must come from local discovery, not from this example:

```ini
[quint]
    driver = usbhid-ups
    port = auto
    desc = Phoenix QUINT UPS
```

Set `/etc/nut/nut.conf`:

```ini
MODE=standalone
```

Restrict `/etc/nut/upsd.conf` to loopback:

```ini
LISTEN 127.0.0.1 3493
```

Read-only `LIST VAR` normally needs no NUT user. Do not add a control-capable user for this gateway. The NUT package's udev rules should grant the driver access; after editing rules, use `udevadm control --reload` and reconnect the UPS. Diagnose ownership with `journalctl -u nut-driver@quint` (unit naming varies by NUT package) and test locally:

```bash
sudo systemctl restart nut-server
upsc quint@127.0.0.1
ss -ltn | grep 3493
```

`upsd` must not listen on the intranet or automation interface. Configure `collectors.nut.ups` to the exact NUT section name (`quint` above), not a display label or USB address.

## WAGO SNMPv3

The sample targets `192.168.1.11:161`, SNMPv3 authPriv, SHA/SHA1, and DES. Replace the account name and secret files with the site's provisioned monitoring account. The historical account name `readonly` is not a security guarantee: the device was observed accepting a write under that account. Network isolation and the absence of a write code path are therefore mandatory.

Normal polling reads only the fixed `collectors.wago.oids` list. Neither query parameters nor request bodies alter it. The WAGO intentionally has no default gateway; keep SNMP local to the automation VLAN. Configure the RevPi with an address that reaches WAGO directly and an intranet-facing route only as required for HTTPS, but do not enable forwarding.

Verify from the RevPi with the distribution SNMP tools only during commissioning, entering credentials interactively or through protected files according to local policy. Never paste passphrases into shared shell history. Once the gateway works, remove direct Zabbix-to-WAGO SNMP routing as described under Firewall migration.

## Configuration and secrets

[`configs/config.example.yaml`](configs/config.example.yaml) is complete. YAML unknown fields, extra documents, malformed durations/OIDs, unsupported SNMP modes, missing references, insecure secret modes, and invalid TLS keypairs make `--check` fail. Secret paths must be regular files rather than symlinks or devices. Secrets may be owner-readable or owner/group-readable, but never accessible to “other” and never group-writable. Bearer tokens must contain 16–4096 non-whitespace bytes; SNMPv3 passphrases must contain 8–255 bytes. Secret files are read through one bounded descriptor and may end in one line ending, but may not contain embedded control characters.

Important fields:

- `poll_interval`: independent cadence for that source.
- `stale_after`: age after which a successful cached result is stale; it must be at least the poll interval.
- `timeout`: NUT connection/deadline or each GoSNMP request timeout.
- `oids`: fixed normal-poll allowlist.
- `discovery.root_oids`: fixed BulkWalk roots used once at startup/reload when discovery is enabled.
- `discovery.max_objects`: hard result bound; exceeding it aborts discovery without publishing partial results (default `2048`, maximum `10000`).
- `discovery.timeout`: hard time limit for one discovery operation (default `2m`, maximum `10m`).
- `health_public`: permits only the minimal health object without bearer auth.
- `max_header_bytes`: bounds request headers; all application routes accept GET only.

Validate as the service user because that checks its actual read permissions:

```bash
sudo -u automation-gateway /usr/local/sbin/automation-gateway --config /etc/automation-gateway/config.yaml --check
```

On SIGHUP, a complete candidate is parsed, semantically validated, all secret and metadata files are loaded, the TLS pair is verified, and collector objects are prepared before activation. Failure preserves the entire active configuration and certificate. A successful reload restarts collector loops and their in-memory snapshots. Listener address, server timeouts/header limit, shutdown timeout, and public-health policy are immutable while running; changing one makes reload fail and requires a restart. Token, TLS files, OIDs, source timing, credentials, and enabled collectors are reloadable.

## MIB metadata and discovery

GoSNMP is not a MIB compiler. Convert reviewed vendor MIBs offline with a trusted MIB tool, then commit/deploy compact JSON metadata in this schema:

```json
[
  {
    "oid": "1.3.6.1.2.1.1.3.0",
    "name": "sysUpTime",
    "type": "TimeTicks",
    "unit": "centiseconds",
    "description": "Device uptime",
    "enum": {"1": "example-state"}
  }
]
```

The runtime strictly rejects unknown JSON fields and duplicate OIDs. Metadata enriches values and discovery; it does not cause an OID to be polled. Review MIB licensing before redistributing vendor files or generated descriptions.

When `discovery.enabled` is true, the collector runs one explicit BulkWalk at source startup or successful reload and caches definitions. `GET /api/v1/sources/wago/discovery` returns that cache and never starts a walk. Full walks do not occur on normal polls or Zabbix requests. Keep roots narrow where possible and move only approved OIDs into the normal allowlist. The example metadata file contains only standard illustrative objects, not a vendor MIB.

## TLS, LEGO, and reload

The Go server terminates TLS with a minimum of TLS 1.2 and Go's secure default cipher policy. It does not implement ACME. LEGO obtains and renews the host leaf certificate and matching private key. Configure the exact LEGO-generated certificate/full-chain and key paths; a CA root or intermediate alone is not a server certificate.

Issue the certificate using the site's already selected LEGO challenge/provider. Provider credentials are environment-specific and must use LEGO's protected credential mechanism; do not place them in gateway YAML. Ensure the service group can read the resulting files/directories without granting broader access:

```bash
sudo chgrp automation-gateway /etc/lego/certificates/monitor.example.invalid.crt /etc/lego/certificates/monitor.example.invalid.key
sudo chmod 0640 /etc/lego/certificates/monitor.example.invalid.crt /etc/lego/certificates/monitor.example.invalid.key
sudo -u automation-gateway /usr/local/sbin/automation-gateway --config /etc/automation-gateway/config.yaml --check
```

After a successful LEGO renewal, the deployment hook must run:

```bash
sudo systemctl reload automation-gateway
sudo journalctl -u automation-gateway -n 20 --no-pager
```

Do not reload after a failed renewal. On HUP the daemon loads and parses the complete new keypair, verifies that the leaf is currently valid, is not a CA, and permits TLS server authentication, then atomically swaps the pointer used by `tls.Config.GetCertificate`. Existing connections continue; new handshakes receive the new pair. An incomplete, mismatched, expired, not-yet-valid, or unsuitable pair leaves the last-known-good certificate active. Verify serials around renewal:

```bash
openssl s_client -connect monitor.example.invalid:443 -servername monitor.example.invalid </dev/null 2>/dev/null | openssl x509 -noout -serial -enddate
sudo systemctl reload automation-gateway
openssl s_client -connect monitor.example.invalid:443 -servername monitor.example.invalid </dev/null 2>/dev/null | openssl x509 -noout -serial -enddate
```

## API reference

All responses are JSON with `Content-Type: application/json`, `X-Content-Type-Options: nosniff`, and `Cache-Control: no-store`. Except for optionally public health, send `Authorization: Bearer <token>`. The authentication scheme is case-insensitive, exactly one Authorization header is accepted, and the token comparison is constant-time after hashing. Common statuses are `200`, `401` (missing/invalid bearer), `404` (unknown route/source), and `405` (anything except GET). Errors have `{"error":"..."}` and do not expose collector or secret details.

### Health

`GET /api/v1/health` returns:

```json
{"status":"ok","version":"1.0.0","certificate_not_after":"2027-01-01T00:00:00Z"}
```

- `ok` / HTTP 200: every configured source is available and fresh.
- `degraded` / HTTP 200: one or more sources are unavailable or stale.
- `failed` / HTTP 503: no sources are configured.

It never returns addresses, credentials, source names, errors, or metrics and may be public if configured.

### Sources

`GET /api/v1/sources` lists source name, driver, availability, and stale state. `GET /api/v1/sources/{name}` returns a full snapshot:

```json
{
  "name": "wago",
  "driver": "snmp",
  "available": false,
  "stale": true,
  "last_attempt": "2026-01-02T03:04:05Z",
  "last_success": "2026-01-02T03:03:30Z",
  "poll_duration_ns": 5000000000,
  "error": "collector unavailable",
  "metrics": [
    {"name":"sysUpTime","value":123,"value_type":"counter","unit":"centiseconds","timestamp":"2026-01-02T03:03:30Z","labels":{"oid":"1.3.6.1.2.1.1.3.0"}}
  ]
}
```

Source failure remains HTTP 200 because the response is a valid snapshot; use `available`, `stale`, and timestamps. `error` is intentionally sanitized. No successful poll yet means an empty metric array, `available:false`, and `stale:true`.

`GET /api/v1/sources/{name}/metrics` returns the same freshness fields plus only metrics. `GET /api/v1/sources/{name}/discovery` returns `{"source":"wago","discovery":[...]}`. Discovery can be empty while its startup walk is pending or after a failed walk; requesting the endpoint does not contact the device.

`GET /api/v1/metrics` returns an object keyed by source containing all snapshots, suitable for one Zabbix master item. Compatibility aliases are:

- `GET /api/v1/quint/status`
- `GET /api/v1/quint/metrics`
- `GET /api/v1/wago/status`
- `GET /api/v1/wago/metrics`
- `GET /api/v1/wago/discovery`

All aliases have the same authentication, schemas, stale semantics, and status codes as their source-oriented equivalents.

Example requests, with the token read without displaying it:

```bash
read -r TOKEN < /etc/automation-gateway/token
curl --fail-with-body --silent --show-error \
  --header "Authorization: Bearer ${TOKEN}" \
  https://monitor.example.invalid/api/v1/sources/wago/metrics
curl --fail-with-body --silent --show-error \
  --header "Authorization: Bearer ${TOKEN}" \
  https://monitor.example.invalid/api/v1/metrics
unset TOKEN
```

Do not use `--insecure` in production; install the proper ACME trust chain instead.

## Zabbix 7.4 example

Create one **HTTP agent** master item:

- Name: `Automation gateway snapshot`
- Key: `automation.gateway.snapshot`
- URL: `https://monitor.example.invalid/api/v1/metrics`
- Request method: `GET`
- Header: `Authorization: Bearer {$AUTOMATION_GATEWAY_TOKEN}`
- Update interval: `30s` (do not poll faster than the shortest useful collector interval)
- Timeout: `10s`, greater than expected LAN/TLS latency but less than the item interval
- Type of information: `Text`
- Required status codes: `200`
- Verify peer and host: enabled

Store `{$AUTOMATION_GATEWAY_TOKEN}` as a **secret macro** at the narrowest appropriate template/host scope. The gateway does not call the Zabbix API.

Create dependent items from the master. Examples:

- WAGO availability: key `automation.gateway.wago.available`, JSONPath `$.wago.available`, type `Numeric (unsigned)`, preprocessing JavaScript if Boolean conversion is required: `return value === 'true' ? 1 : 0;`.
- WAGO uptime: key `automation.gateway.wago.uptime`, JSONPath `$.wago.metrics[?(@.name == 'sysUpTime')].value.first()`, type `Numeric (unsigned)`.
- QUINT charge: key `automation.gateway.quint.charge`, JSONPath `$.quint.metrics[?(@.name == 'battery.charge')].value.first()`, type `Numeric (float)`.

Add trigger logic for `$.wago.stale`, `$.wago.available`, and master-item unsupported/no-data conditions. A stale metric remains intentionally present; never treat its numeric value as current without checking source freshness.

## Service operation and logging

```bash
sudo systemctl status automation-gateway
sudo journalctl -u automation-gateway --since today
sudo systemctl reload automation-gateway
sudo systemctl restart automation-gateway
sudo systemctl stop automation-gateway
```

Logs are one JSON object per line through stdout/stderr for journald. Levels are `debug`, `info`, `warn`, and `error`. INFO records lifecycle, reload, and state transitions, not every successful poll. Errors are sanitized by the API; logs identify source and operation but do not include token/passphrase values.

SIGINT and SIGTERM stop accepting requests, allow bounded HTTP shutdown, cancel collector/discovery contexts, wait for them, and close network connections. SIGHUP only reloads. The unit restarts unexpected failures.

Resource design is bounded: snapshots hold only current configured metrics/discovery, there is no history/database or unbounded queue, normal SNMP uses GET rather than walks, and one goroutine is used per active poller plus a temporary discovery goroutine. Measure on the actual RevPi after commissioning:

```bash
systemctl show automation-gateway -p MemoryCurrent -p CPUUsageNSec -p TasksCurrent
/usr/bin/time -v /usr/local/sbin/automation-gateway --config /etc/automation-gateway/config.yaml --check
```

Build-host measurements are not a substitute for RevPi steady-state measurements because architecture, Go runtime, active metrics, and network behavior differ.

## Firewall migration

Do not install example firewall rules blindly: the final RevPi address is site-specific. During commissioning, allow only Zabbix `192.168.2.232` to the chosen RevPi HTTPS address/port across `wago-gw`. Keep forwarding default-deny. The RevPi needs local outbound UDP/161 to WAGO and loopback TCP/3493 to upsd; it must not route packets.

After HTTPS monitoring is verified, remove both legacy direct-monitoring rules from `wago-gw`:

- Zabbix `192.168.2.232` to WAGO `192.168.1.11` UDP/161 accept.
- Matching SNAT to `192.168.1.254`.

Do not alter the independent admin timeout-set access for HTTP/CODESYS or required Modbus/TCP. Verify the active nftables ruleset and connectivity before and after the change, and retain an out-of-band rollback path.

## Update, rollback, and removal

Start updates only from a clean worktree and keep versioned binaries for rollback:

```bash
cd HomeAuthMonitorGW
git status --short
git pull --ff-only
go mod download
go test ./...
go vet ./...
go build -trimpath -o automation-gateway ./cmd/gateway
sudo -u automation-gateway ./automation-gateway --config /etc/automation-gateway/config.yaml --check
sudo cp /usr/local/sbin/automation-gateway /usr/local/sbin/automation-gateway.previous
sudo install -o root -g root -m 0755 automation-gateway /usr/local/sbin/automation-gateway
sudo install -o root -g root -m 0644 systemd/automation-gateway.service /etc/systemd/system/automation-gateway.service
sudo systemctl daemon-reload
sudo systemctl restart automation-gateway
sudo systemctl status automation-gateway
sudo journalctl -u automation-gateway -n 50 --no-pager
```

Updates preserve `/etc/automation-gateway`, `/etc/lego`, NUT configuration, and secret files. If verification fails, rollback:

```bash
sudo install -o root -g root -m 0755 /usr/local/sbin/automation-gateway.previous /usr/local/sbin/automation-gateway
sudo systemctl restart automation-gateway
sudo systemctl status automation-gateway
```

For removal:

```bash
sudo systemctl disable --now automation-gateway
sudo rm /etc/systemd/system/automation-gateway.service
sudo systemctl daemon-reload
sudo rm /usr/local/sbin/automation-gateway /usr/local/sbin/automation-gateway.previous
```

Review and separately remove `/etc/automation-gateway`, secrets, NUT configuration, LEGO material, and the service account only when no rollback or other service needs them. Those deletions are intentionally not in the copyable removal block.

## Troubleshooting

- **`--check` says unknown field:** correct the spelling; fields are intentionally strict.
- **Secret permissions rejected:** use owner/group read only (`0640`) and verify every parent directory is traversable by `automation-gateway`.
- **TLS keypair error:** ensure the file is the host leaf/full chain and that its private key matches. A CA bundle alone is insufficient.
- **Reload failed:** inspect journald, correct the candidate, rerun `--check`, then reload. The prior config/certificate remains active.
- **Listener change rejected on reload:** restart; listener/server policies are immutable at runtime.
- **NUT unavailable:** run `upsc`, inspect NUT driver/upsd units, confirm loopback listen and exact UPS name, then inspect USB/udev permissions.
- **SNMP unavailable:** verify direct automation-VLAN routing, UDP/161, account/authPriv settings, device engine-ID behavior, and timeout. Collector errors do not stop the daemon.
- **Discovery empty:** wait for startup discovery, inspect logs, reduce roots, and verify metadata JSON separately. API reads the cache and never launches a walk.
- **Values stale but present:** this is last-known-good behavior. Check `last_attempt`, `last_success`, `available`, and source logs.
- **401:** verify the Zabbix secret macro and token file content; do not log or print either.
- **503 health:** no sources are enabled. Individual source outages produce `degraded` with HTTP 200.
- **Unit hardening issue:** run `systemd-analyze verify` and inspect journal messages. Do not remove protections wholesale; determine the exact unsupported directive or blocked access first.

## Security considerations

- Keep SNMPv3 SHA1/DES confined to the automation VLAN. HTTPS protects the intranet side.
- Do not trust an SNMP username such as `readonly` as authorization evidence.
- No production code invokes SNMP SET or accepts OIDs/operations from clients. NUT emits only `LIST VAR` and accepts no API-selected command.
- Keep upsd loopback-only, forwarding disabled, the WAGO without a default gateway, and inter-VLAN policy default-deny.
- Rotate bearer and SNMP secrets using protected files, validate, and reload. Never place them in YAML, command lines, tickets, source control, or logs.
- Restrict certificate/key and secret access to root and the service group. The process runs without root and has only bind-service capability.
- Public health is deliberately minimal. Protect it too by setting `health_public: false` when external monitoring does not require anonymous liveness.
- Discovery roots and normal OIDs are administrator configuration, never API input. Review newly discovered objects before adding them to the allowlist.
- Preserve last-known-good config/certificate behavior: never automate a restart after a failed validation or renewal.
- Keep Go and pinned dependencies patched, review `go.sum` changes, and rerun native plus target-architecture builds before deployment.

## License

See [LICENSE](LICENSE).
