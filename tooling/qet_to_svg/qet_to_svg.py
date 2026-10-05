#!/usr/bin/env python3
"""QElectroTech .elmt -> SVG, Python 3.8+, ohne Zusatzpakete.

Aufruf: python3 qet_to_svg.py datei.elmt
        python3 qet_to_svg.py *.elmt -o svg
        python3 qet_to_svg.py 'elemente/**/*.elmt' -o svg

Unterstützt polygon (auch offen), rect, ellipse, text, dynamic_text.
Unbekannte Zeichenobjekte führen absichtlich zu einem Fehler pro Datei.
Schriftmetriken und dynamische Textpositionen sind Näherungen; für ähnliche
Schriftbilder sollte die im Element genannte Schrift installiert sein.
"""
import argparse
import glob
import math
from pathlib import Path
import re
import sys
import xml.etree.ElementTree as ET

NS = 'http://www.w3.org/2000/svg'
ET.register_namespace('', NS)
COLORS = {'white': '#ffffff', 'black': '#000000', 'lightgray': '#c0c0c0',
          'gray': '#a0a0a4', 'green': '#00ff00', 'red': '#ff0000',
          'orange': '#ff8000', 'blue': '#0000ff', 'yellow': '#ffff00',
          'cyan': '#00ffff', 'magenta': '#ff00ff', 'brun': '#612c00',
          'purple': '#881ca8', 'none': 'none'}

def tag(name):
    return '{%s}%s' % (NS, name)

def number(value):
    value = float(value)
    if not math.isfinite(value):
        raise ValueError('Nicht endliche Koordinate oder Größe')
    return value

def fmt(value):
    return format(number(value), '.10g')

def color(value):
    if value in COLORS:
        return COLORS[value]
    if re.fullmatch(r'#[0-9a-fA-F]{6}', value):
        return value
    raise ValueError('Unbekannte Farbe: ' + value)

def convert(source, margin=2, dpi=96):
    root = ET.parse(source).getroot()
    description = root.find('description')
    if root.tag != 'definition' or description is None:
        raise ValueError('Keine QElectroTech-Definition mit <description>')
    svg = ET.Element(tag('svg'), {'version': '1.1'})
    ET.SubElement(svg, tag('title')).text = root.findtext('names/name') or source.stem
    ET.SubElement(svg, tag('desc')).text = (root.findtext('informations') or '') + '\nConverted from QElectroTech. Thin stroke: 0.5 units. Text metrics approximated.'
    group = ET.SubElement(svg, tag('g'), {'stroke-linejoin': 'bevel', 'stroke-linecap': 'square'})
    bounds = []
    information = {n.get('name'): n.text or '' for n in root.findall('elementInformations/elementInformation')}
    for node in description:
        a, kind = node.attrib, node.tag
        if kind in ('text', 'dynamic_text'):
            dynamic = kind == 'dynamic_text'
            content = a.get('text', '') if not dynamic else node.findtext('text') or ''
            if dynamic and a.get('text_from') == 'ElementInfo':
                content = information.get(node.findtext('info_name'), '')
            elif dynamic and a.get('text_from') not in (None, 'UserText'):
                raise ValueError('Nicht unterstützte dynamische Textquelle: ' + a['text_from'])
            if not content:
                continue
            if '<' in content and dynamic:
                raise ValueError('Dynamischer HTML-Text wird nicht unterstützt')
            x, y = number(a.get('x', 0)), number(a.get('y', 0))
            font = a.get('font', 'sans-serif,9,-1,5,50,0').split(',')
            size = number(font[1]) * dpi / 72
            if size <= 0 and len(font) > 2:
                size = number(font[2])
            if size <= 0:
                raise ValueError('Ungültige Schriftgröße')
            angle = number(a.get('rotation', 0))
            attrs = {'x': fmt(x), 'y': fmt(y), 'font-family': font[0] + ', Arial, sans-serif', 'font-size': fmt(size), 'fill': color(a.get('color', '#000000'))}
            weight = number(font[4]) if len(font) > 4 else 50
            attrs['font-weight'] = '700' if weight >= 75 else '300' if weight < 50 else '400'
            if len(font) > 5 and font[5] == '1':
                attrs['font-style'] = 'italic'
            if angle:
                attrs['transform'] = 'rotate(%s %s %s)' % (fmt(angle), fmt(x), fmt(y))
            if dynamic:
                # QGraphicsTextItem positions refer to the top left of its box.
                y += size + 4
                x += 4
                attrs.update({'x': fmt(x), 'y': fmt(y)})
                if a.get('frame') == 'true':
                    raise ValueError('Dynamischer Textrahmen wird nicht unterstützt')
            text = ET.SubElement(group, tag('text'), attrs)
            text.set('{http://www.w3.org/XML/1998/namespace}space', 'preserve')
            lines = content.split('\n')
            for index, line in enumerate(lines):
                ET.SubElement(text, tag('tspan'), {'x': fmt(x), 'dy': '0' if index == 0 else fmt(size * 1.2)}).text = line
            rad = math.radians(angle)
            ox, oy = number(a.get('x', 0)), number(a.get('y', 0))
            for px in (x-size*.2, x+max(map(len, lines))*size*1.3):
                for py in (y-size*1.3, y+(len(lines)-1)*size*1.2+size*.4):
                    dx, dy = px-ox, py-oy
                    bounds.append((ox+dx*math.cos(rad)-dy*math.sin(rad), oy+dx*math.sin(rad)+dy*math.cos(rad)))
            continue
        if kind not in ('polygon', 'rect', 'ellipse'):
            raise ValueError('Nicht unterstütztes Zeichenobjekt: ' + kind)
        styles = dict(part.strip().split(':', 1) for part in a.get('style', '').split(';') if part.strip())
        widths = {'none': 0, 'thin': .5, 'normal': 1, 'hight': 2, 'eleve': 5}
        width = widths[styles.get('line-weight', 'normal')]
        attrs = {'fill': color(styles.get('filling', 'none')), 'stroke': color(styles.get('color', 'black')), 'stroke-width': fmt(width)}
        if width == 0:
            attrs['stroke'] = 'none'
        dash = styles.get('line-style', 'normal')
        if dash != 'normal':
            attrs['stroke-dasharray'] = {'dashed': '4 2', 'dotted': '1 2', 'dashdotted': '4 2 1 2'}[dash]
        if kind == 'polygon':
            indices = sorted(int(k[1:]) for k in a if re.fullmatch(r'x\d+', k))
            points = [(number(a['x'+str(i)]), number(a['y'+str(i)])) for i in indices]
            if len(points) < 2:
                raise ValueError('Polygon hat weniger als zwei Punkte')
            attrs['points'] = ' '.join(fmt(x)+','+fmt(y) for x, y in points)
            if a.get('closed') == 'false':
                kind = 'polyline'
        else:
            x, y, w, h = (number(a[k]) for k in ('x', 'y', 'width', 'height'))
            if w < 0 or h < 0:
                raise ValueError('Negative Breite oder Höhe')
            points = [(x, y), (x+w, y+h)]
            if kind == 'rect':
                attrs.update({k: fmt(a[k]) for k in ('x', 'y', 'width', 'height', 'rx', 'ry') if k in a})
            else:
                attrs.update({'cx': fmt(x+w/2), 'cy': fmt(y+h/2), 'rx': fmt(w/2), 'ry': fmt(h/2)})
        ET.SubElement(group, tag(kind), attrs)
        pad = width / math.sqrt(2) if attrs['stroke'] != 'none' else 0
        for x, y in points:
            bounds.extend([(x-pad, y-pad), (x+pad, y+pad)])
    if not bounds:
        raise ValueError('Keine sichtbare Geometrie vorhanden')
    x0, y0 = min(p[0] for p in bounds)-margin, min(p[1] for p in bounds)-margin
    x1, y1 = max(p[0] for p in bounds)+margin, max(p[1] for p in bounds)+margin
    width, height = max(x1-x0, .01), max(y1-y0, .01)
    svg.set('viewBox', ' '.join(fmt(v) for v in (x0, y0, width, height)))
    svg.set('width', fmt(width))
    svg.set('height', fmt(height))
    return ET.tostring(svg, encoding='utf-8', xml_declaration=True)

def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('files', nargs='+', help='Dateien oder Wildcards; Dateiendung ist beliebig')
    parser.add_argument('-o', '--output-dir', type=Path, help='Ausgabeordner; Standard: neben der Eingabedatei')
    parser.add_argument('--force', action='store_true', help='Vorhandene SVG-Dateien überschreiben')
    parser.add_argument('--margin', type=float, default=2, help='Rand in Zeichnungseinheiten (Standard: 2)')
    parser.add_argument('--dpi', type=float, default=96, help='Umrechnung von Schriftpunkten (Standard: 96)')
    args = parser.parse_args(argv)
    if not math.isfinite(args.margin) or args.margin < 0 or not math.isfinite(args.dpi) or args.dpi <= 0:
        parser.error('Rand muss endlich und >= 0 sein; DPI endlich und > 0')
    sources, seen, errors = [], set(), 0
    for pattern in args.files:
        matches = [pattern] if Path(pattern).is_file() else sorted(glob.glob(pattern, recursive=True))
        if not matches:
            print('FEHLER: Kein Treffer: ' + pattern, file=sys.stderr)
            errors += 1
        for match in matches:
            source = Path(match).resolve()
            if source not in seen:
                sources.append(source)
                seen.add(source)
    destinations = {}
    for source in sources:
        dest = (args.output_dir or source.parent) / (source.stem + '.svg')
        destinations.setdefault(dest.resolve(), []).append(source)
    success = 0
    for dest, inputs in destinations.items():
        if len(inputs) > 1:
            print('FEHLER: Mehrere Quellen ergeben dieselbe Ausgabe %s: %s' % (dest, ', '.join(map(str, inputs))), file=sys.stderr)
            errors += len(inputs)
            continue
        source = inputs[0]
        try:
            if dest in seen:
                raise ValueError('Ausgabe würde eine Eingabedatei überschreiben')
            data = convert(source, args.margin, args.dpi)
            dest.parent.mkdir(parents=True, exist_ok=True)
            with dest.open('wb' if args.force else 'xb') as stream:
                stream.write(data)
            print('OK: %s -> %s' % (source, dest))
            success += 1
        except (OSError, ValueError, KeyError, ET.ParseError) as exc:
            print('FEHLER: %s: %s' % (source, exc), file=sys.stderr)
            errors += 1
    print('%d konvertiert, %d Fehler.' % (success, errors))
    return 1 if errors else 0

if __name__ == '__main__':
    sys.exit(main())
