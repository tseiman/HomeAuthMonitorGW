# Optional catalog SVG files

Place additional trusted WAGO module SVGs in this directory before running
`scripts/install_widget.sh`.

The installer copies every regular, non-symlink `*.svg` file from this directory to
`/var/lib/zabbix/wago_kbus/images/` **only when the destination file does not already
exist**. Existing site-local files are never overwritten.

The default catalog currently references these user-supplied filenames:

- `wago_0750-0400.svg`
- `wago_0750-0501.svg`
- `wago_0750-1405.svg`
- `wago_0750-1504.svg`
- `wago_0750-0652.svg`

`wago_0750-0880.svg`, `wago_0750-0511.svg`, `wago_0750-0600.svg`, and the generic
fallback SVGs are already provided under `assets/img/`.

Only commit SVGs whose provenance and redistribution licence have been verified.
