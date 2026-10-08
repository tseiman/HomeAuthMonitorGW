class CWidgetWagoKbus extends CWidget {
    onInitialize() {
        super.onInitialize();
        this._kbusHandlers = [];
        this._kbusActive   = null;
    }

    setContents(data) {
        this._kbusTeardown();
        super.setContents(data);
        this._kbusSetup();
    }

    onActivate() {
        super.onActivate();
        this._kbusSetup();
    }

    onDeactivate() {
        this._kbusTeardown();
        super.onDeactivate();
    }

    onResize() {
        super.onResize();
        this._kbusReposition();
    }

    onDestroy() {
        this._kbusTeardown();
        super.onDestroy();
    }

    _kbusPanel() {
        return this._body ? this._body.querySelector('.wago-kbus-panel') : null;
    }

    _kbusSetup() {
        const panel = this._kbusPanel();
        if (!panel) return;

        panel.querySelectorAll('.wago-kbus-item').forEach(item => {
            const tooltip = item.querySelector('.wago-kbus-tooltip');
            if (!tooltip) return;

            const onEnter   = ()  => this._kbusShow(item, tooltip, panel);
            const onLeave   = ()  => this._kbusMaybeHide(tooltip);
            const onClick   = ()  => tooltip._wkbPinned
                                       ? this._kbusReturn(tooltip)
                                       : this._kbusPin(item, tooltip, panel);
            const onKeyDown = (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    tooltip._wkbPinned
                        ? this._kbusReturn(tooltip)
                        : this._kbusPin(item, tooltip, panel);
                }
            };
            const onFocusIn  = ()  => this._kbusShow(item, tooltip, panel);
            const onFocusOut = (e) => {
                if (!tooltip.contains(e.relatedTarget)) {
                    this._kbusMaybeHide(tooltip);
                }
            };

            item.addEventListener('pointerenter', onEnter);
            item.addEventListener('pointerleave', onLeave);
            item.addEventListener('click',        onClick);
            item.addEventListener('keydown',      onKeyDown);
            item.addEventListener('focusin',      onFocusIn);
            item.addEventListener('focusout',     onFocusOut);

            this._kbusHandlers.push({item, tooltip,
                onEnter, onLeave, onClick, onKeyDown, onFocusIn, onFocusOut});
        });

        this._kbusDocKeyDown = (e) => {
            if (e.key === 'Escape' && this._kbusActive) {
                e.preventDefault();
                this._kbusReturn(this._kbusActive);
            }
        };
        document.addEventListener('keydown', this._kbusDocKeyDown);

        this._kbusOnResize = () => this._kbusReposition();
        window.addEventListener('resize', this._kbusOnResize);

        this._kbusOnScroll = () => this._kbusReposition();
        document.addEventListener('scroll', this._kbusOnScroll, true);
    }

    _kbusTeardown() {
        this._kbusHandlers.forEach(({item, tooltip,
                onEnter, onLeave, onClick, onKeyDown, onFocusIn, onFocusOut}) => {
            item.removeEventListener('pointerenter', onEnter);
            item.removeEventListener('pointerleave', onLeave);
            item.removeEventListener('click',        onClick);
            item.removeEventListener('keydown',      onKeyDown);
            item.removeEventListener('focusin',      onFocusIn);
            item.removeEventListener('focusout',     onFocusOut);
            if (tooltip._wkbFloat) {
                this._kbusReturn(tooltip);
            }
        });
        this._kbusHandlers = [];

        if (this._kbusDocKeyDown) {
            document.removeEventListener('keydown', this._kbusDocKeyDown);
            this._kbusDocKeyDown = null;
        }
        if (this._kbusOnResize) {
            window.removeEventListener('resize', this._kbusOnResize);
            this._kbusOnResize = null;
        }
        if (this._kbusOnScroll) {
            document.removeEventListener('scroll', this._kbusOnScroll, true);
            this._kbusOnScroll = null;
        }
    }

    _kbusReposition() {
        if (!this._kbusActive || !this._kbusActive._wkbFloat) return;
        const panel = this._kbusPanel();
        const item = this._kbusActive._wkbHome;
        if (!panel || !panel.isConnected || !item || !item.isConnected) {
            this._kbusReturn(this._kbusActive);
            return;
        }
        this._kbusPosition(item, this._kbusActive, panel);
    }

    _kbusShow(item, tooltip, panel) {
        if (tooltip._wkbPinned) return;
        if (this._kbusActive && this._kbusActive._wkbPinned && this._kbusActive !== tooltip) {
            return;
        }
        this._kbusFloat(item, tooltip, panel);
    }

    _kbusFloat(item, tooltip, panel) {
        if (this._kbusActive && this._kbusActive !== tooltip && !this._kbusActive._wkbPinned) {
            this._kbusReturn(this._kbusActive);
        }

        if (!tooltip._wkbFloat) {
            tooltip._wkbHome = item;
            document.body.appendChild(tooltip);
            tooltip.classList.add('wago-kbus-tooltip--floating');
            tooltip._wkbFloat = true;
        }
        tooltip.style.left       = '-9999px';
        tooltip.style.top        = '0';
        tooltip.style.opacity    = '0';
        tooltip.style.visibility = 'visible';

        this._kbusActive = tooltip;
        item.setAttribute('aria-expanded', 'true');

        this._kbusEnsureConnector(tooltip);
        this._kbusPosition(item, tooltip, panel);
        tooltip.style.opacity = '1';
    }

    _kbusMaybeHide(tooltip) {
        if (tooltip._wkbPinned) return;
        this._kbusReturn(tooltip);
    }

    _kbusReturn(tooltip) {
        if (!tooltip._wkbFloat) return;

        tooltip.style.opacity    = '';
        tooltip.style.visibility = '';
        tooltip.style.left       = '';
        tooltip.style.top        = '';
        tooltip.style.maxWidth   = '';
        tooltip.style.minWidth   = '';
        tooltip.style.maxHeight  = '';
        tooltip.style.overflowY  = '';
        tooltip.classList.remove('wago-kbus-tooltip--floating');
        tooltip.classList.remove('wago-kbus-tooltip--pinned');
        tooltip.setAttribute('role', 'tooltip');
        tooltip._wkbFloat  = false;
        tooltip._wkbPinned = false;

        if (tooltip._wkbClose) {
            tooltip._wkbClose.remove();
            tooltip._wkbClose = null;
        }
        if (tooltip._wkbConnector) {
            tooltip._wkbConnector.svg.remove();
            tooltip._wkbConnector = null;
        }

        if (this._kbusActive === tooltip) {
            this._kbusActive = null;
        }

        const home = tooltip._wkbHome;
        tooltip._wkbHome = null;
        if (home) {
            home.setAttribute('aria-expanded', 'false');
            home.appendChild(tooltip);
        }

        const returnFocus = tooltip._wkbReturnFocus;
        tooltip._wkbReturnFocus = null;
        if (returnFocus && document.contains(returnFocus)) {
            returnFocus.focus();
        }
    }

    _kbusPin(item, tooltip, panel) {
        if (this._kbusActive && this._kbusActive !== tooltip) {
            const prev = this._kbusActive;
            prev._wkbReturnFocus = null;
            this._kbusReturn(prev);
        }

        this._kbusFloat(item, tooltip, panel);
        tooltip._wkbPinned      = true;
        tooltip._wkbReturnFocus = item;
        tooltip.classList.add('wago-kbus-tooltip--pinned');
        tooltip.setAttribute('role', 'dialog');

        if (!tooltip._wkbClose) {
            const btn = document.createElement('button');
            btn.type        = 'button';
            btn.textContent = '×';
            btn.setAttribute('aria-label', 'Close details');
            btn.className = 'wago-kbus-tooltip-close';
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                this._kbusReturn(tooltip);
            });
            tooltip.appendChild(btn);
            tooltip._wkbClose = btn;
        }

        // Reposition after pin class and close button have been added to the layout.
        this._kbusPosition(item, tooltip, panel);
    }

    _kbusEnsureConnector(tooltip) {
        if (tooltip._wkbConnector) return;

        const ns = 'http://www.w3.org/2000/svg';
        const svg = document.createElementNS(ns, 'svg');
        const line = document.createElementNS(ns, 'line');
        const arrow = document.createElementNS(ns, 'polygon');

        svg.classList.add('wago-kbus-tooltip-connector');
        svg.setAttribute('aria-hidden', 'true');
        line.classList.add('wago-kbus-tooltip-connector-line');
        arrow.classList.add('wago-kbus-tooltip-connector-arrow');
        svg.appendChild(line);
        svg.appendChild(arrow);
        document.body.appendChild(svg);

        tooltip._wkbConnector = {svg, line, arrow};
    }

    _kbusPositionConnector(item, tooltip, panel) {
        const connector = tooltip._wkbConnector;
        if (!connector || !item.isConnected || !panel.isConnected) return;

        const itemRect = item.getBoundingClientRect();
        const tipRect = tooltip.getBoundingClientRect();
        const panelRect = panel.getBoundingClientRect();
        const vw = document.documentElement.clientWidth;
        const vh = document.documentElement.clientHeight;
        const anchorX = itemRect.left + itemRect.width / 2;
        const anchorY = itemRect.top + itemRect.height / 2;
        const safeLeft = Math.max(panelRect.left, 0);
        const safeTop = Math.max(panelRect.top, 0);
        const safeRight = Math.min(panelRect.right, vw);
        const safeBottom = Math.min(panelRect.bottom, vh);

        const valid = [anchorX, anchorY, tipRect.left, tipRect.top,
            tipRect.right, tipRect.bottom].every(Number.isFinite);
        const anchorVisible = anchorX >= safeLeft && anchorX <= safeRight
            && anchorY >= safeTop && anchorY <= safeBottom;
        const anchorInsideTooltip = anchorX >= tipRect.left && anchorX <= tipRect.right
            && anchorY >= tipRect.top && anchorY <= tipRect.bottom;

        if (!valid || !anchorVisible || anchorInsideTooltip) {
            connector.svg.style.display = 'none';
            return;
        }

        const tipCenterX = tipRect.left + tipRect.width / 2;
        const tipCenterY = tipRect.top + tipRect.height / 2;
        const dx = anchorX - tipCenterX;
        const dy = anchorY - tipCenterY;
        const halfW = tipRect.width / 2;
        const halfH = tipRect.height / 2;
        const edgeScale = 1 / Math.max(Math.abs(dx) / halfW, Math.abs(dy) / halfH);
        const startX = tipCenterX + dx * edgeScale;
        const startY = tipCenterY + dy * edgeScale;
        const length = Math.hypot(anchorX - startX, anchorY - startY);

        if (!Number.isFinite(length) || length < 4) {
            connector.svg.style.display = 'none';
            return;
        }

        const ux = (anchorX - startX) / length;
        const uy = (anchorY - startY) / length;
        const arrowLength = Math.min(8, length / 2);
        const arrowHalfWidth = 4;
        const baseX = anchorX - ux * arrowLength;
        const baseY = anchorY - uy * arrowLength;
        const perpX = -uy * arrowHalfWidth;
        const perpY = ux * arrowHalfWidth;

        connector.line.setAttribute('x1', String(startX));
        connector.line.setAttribute('y1', String(startY));
        connector.line.setAttribute('x2', String(anchorX));
        connector.line.setAttribute('y2', String(anchorY));
        connector.arrow.setAttribute('points', [
            `${anchorX},${anchorY}`,
            `${baseX + perpX},${baseY + perpY}`,
            `${baseX - perpX},${baseY - perpY}`
        ].join(' '));
        connector.svg.style.display = '';
    }

    _kbusPosition(item, tooltip, panel) {
        const panelRect = panel.getBoundingClientRect();
        const vw        = document.documentElement.clientWidth;
        const vh        = document.documentElement.clientHeight;
        const margin    = 8;

        // Intersect panel rect with viewport so positioning stays on-screen.
        const safeLeft   = Math.max(panelRect.left,   0);
        const safeTop    = Math.max(panelRect.top,    0);
        const safeRight  = Math.min(panelRect.right,  vw);
        const safeBottom = Math.min(panelRect.bottom, vh);
        const safeW      = Math.max(0, safeRight  - safeLeft);
        const safeH      = Math.max(0, safeBottom - safeTop);

        const itemRect = item.getBoundingClientRect();

        // Clamp both dimensions to the visible panel area. Inline min-width is
        // required because the stylesheet's normal 140px minimum would otherwise
        // override max-width in very narrow dashboard columns.
        const availableW = Math.max(1, safeW - 2 * margin);
        const availableH = Math.max(1, safeH - 2 * margin);
        const maxW = Math.min(360, availableW);
        tooltip.style.maxWidth = maxW + 'px';
        tooltip.style.minWidth = Math.min(140, maxW) + 'px';
        tooltip.style.maxHeight = availableH + 'px';
        tooltip.style.overflowY = 'auto';

        // Measure natural tooltip size (visible, off-screen at left:-9999px).
        const tr = tooltip.getBoundingClientRect();
        const tw = Math.min(tr.width || maxW, availableW);
        const th = Math.min(tr.height || availableH, availableH);

        // Horizontal: prefer right of item, then left, then center-clamp inside safe area.
        let left;
        let beside = true;
        if (itemRect.right + margin + tw <= safeRight - margin) {
            left = itemRect.right + margin;
        } else if (itemRect.left - margin - tw >= safeLeft + margin) {
            left = itemRect.left - margin - tw;
        } else {
            beside = false;
            left = itemRect.left + itemRect.width / 2 - tw / 2;
            left = Math.max(safeLeft + margin, Math.min(safeRight - tw - margin, left));
        }

        const anchorY = itemRect.top + itemRect.height / 2;
        let top;
        if (beside) {
            top = anchorY - th / 2;
        } else {
            const connectorGap = 12;
            const above = anchorY - th - connectorGap;
            const below = anchorY + connectorGap;
            if (above >= safeTop + margin) {
                top = above;
            } else if (below + th <= safeBottom - margin) {
                top = below;
            } else {
                top = anchorY - th / 2;
            }
        }
        top = Math.max(safeTop + margin, Math.min(safeBottom - th - margin, top));

        tooltip.style.left = left + 'px';
        tooltip.style.top  = top  + 'px';
        this._kbusPositionConnector(item, tooltip, panel);
    }
}
