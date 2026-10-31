// Interface scale: one percentage drives the whole app.
//
// The non-standard CSS `zoom` property scales layout (and every fixed px in
// styles.css) without a stylesheet rewrite. It has one catch: under zoom,
// getBoundingClientRect() returns *visual* pixels while inline top/left are
// applied in *layout* pixels, so Floating UI (Radix popovers, dropdowns and
// dialogs) would drift by exactly the zoom factor. Dividing every measured
// rect by the active zoom puts both coordinate systems back in sync.
const BASE_SCALE = 1.25 // v2.1 design baseline: 100% in settings equals this
let patched = false

function activeZoom(): number {
  const value = Number(document.documentElement.style.zoom)
  return Number.isFinite(value) && value > 0 ? value : 1
}

function installRectPatch() {
  if (patched) return
  patched = true
  const original = Element.prototype.getBoundingClientRect
  Element.prototype.getBoundingClientRect = function (this: Element) {
    const rect = original.call(this)
    const zoom = activeZoom()
    if (zoom === 1) return rect
    return new DOMRect(rect.x / zoom, rect.y / zoom, rect.width / zoom, rect.height / zoom)
  }
}

export function applyInterfaceScale(percent: number) {
  installRectPatch()
  document.documentElement.style.zoom = String((percent / 100) * BASE_SCALE)
}
