# qet_to_svg — provenance and licensing

## Tool licensing

`qet_to_svg.py` is project-internal tooling (part of the HomeAuthMonitorGW repository) written
to convert QElectroTech `.elmt` element files to plain SVG.  It is distributed under the same
MIT licence as the rest of this repository (see `LICENSE` in the repository root).

The tool has no runtime dependencies beyond the Python 3.8+ standard library.

---

## SVG asset licensing

The SVG files shipped in `zabbix/modules/wago_kbus/assets/img/` are derivatives of element
definitions from the **qelectrotech-elements** community library:

> qelectrotech-elements — <https://github.com/qelectrotech/qelectrotech-elements>
> Typically distributed under the **Creative Commons Attribution 4.0 International (CC BY 4.0)**
> licence; individual element files may carry different terms — verify the licence header
> in each `.elmt` source file before redistribution.
> Copyright © QElectroTech contributors.

### Attribution (required by CC BY 4.0)

The WAGO 750-series `.elmt` source files were obtained from the qelectrotech-elements
repository and converted using `qet_to_svg.py`.  Any redistribution of the converted SVG
files should retain this attribution notice.  Confirm the exact licence in each source
`.elmt` file before redistribution (typically CC BY 4.0:
<https://creativecommons.org/licenses/by/4.0/>).

### Known source paths (within qelectrotech-elements)

| SVG file shipped                     | Likely upstream path in qelectrotech-elements          |
|--------------------------------------|--------------------------------------------------------|
| `wago_0750-0880.svg`                 | `sources/industrial/WAGO/750-880/`                     |
| `wago_0750-0511.svg`                 | `sources/industrial/WAGO/750-511/`                     |
| `wago_0750-xxxx_controller.svg`      | Project-created generic fallback (not from upstream)   |
| `wago_0750-xxxx_modul.svg`           | Project-created generic fallback (not from upstream)   |
| `wago_0750-xxxx.svg`                 | Project-created generic fallback (not from upstream)   |

The generic fallback files (`*_controller.svg`, `*_modul.svg`, `*_xxxx.svg`) are
project-original artwork and carry the same MIT licence as the rest of the repository.

---

## Reproducing the conversion

The procedure below fetches a single `.elmt` file without cloning the entire upstream
repository, converts it, and places it in the widget asset directory.

```sh
# 1. Fetch one element file (replace the path with the actual upstream path).
curl -sSL \
  "https://raw.githubusercontent.com/qelectrotech/qelectrotech-elements/master/sources/industrial/WAGO/750-880/element.elmt" \
  -o wago_0750-0880.elmt

# 2. Convert to SVG.
python3 tooling/qet_to_svg/qet_to_svg.py wago_0750-0880.elmt \
  -o zabbix/modules/wago_kbus/assets/img/ --force

# 3. Rename to the expected filename convention if necessary.
#    Format: wago_0750-<4-digit-article>.svg
#    e.g. wago_0750-0880.elmt → wago_0750-0880.svg (done automatically by the tool)

# 4. Verify the SVG renders correctly in a browser.
#    Open test/wago_kbus_preview.html (see tooling/qet_to_svg/HOWTO.md) or use any SVG viewer.
```

To add a new module SVG once the file is in `assets/img/`:

1. Add the article base number → filename mapping to `$SVG_MODULE_MAP` in
   `zabbix/modules/wago_kbus/views/widget.view.php`.
2. Optionally add a human-readable description to `$WAGO_DESCRIPTIONS` in the same file.
3. Re-run the widget tests (`go test ./zabbix/...`) to confirm nothing regressed.

---

## Upstream notices required for redistribution

When distributing any file derived from qelectrotech-elements, include an attribution notice
such as the following (adjust if the source element carries a different licence):

```
Derived from qelectrotech-elements (https://github.com/qelectrotech/qelectrotech-elements)
© QElectroTech contributors — verify licence per element; typically CC BY 4.0
(https://creativecommons.org/licenses/by/4.0/)
```

This notice is included in this file and should be preserved in any downstream copy or
repackaging of the widget.
