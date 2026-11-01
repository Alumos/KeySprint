// Interface scale: one percentage drives the whole app.
//
// The non-standard CSS `zoom` property scales layout (and every fixed px in
// styles.css) without a stylesheet rewrite. It has one catch: under zoom,
// getBoundingClientRect() returns *visual* pixels while inline top/left are
// applied in *layout* pixels, so Floating UI (Radix popovers, dropdowns and
// dialogs) would drift by exactly the zoom factor. Dividing every measured
// rect by the active zoom puts both coordinate systems back in sync.
//
// Phones are excluded on purpose. The console already collapses to one column
// below 760px, and zooming the root there leaves only 1/1.25 of the screen for
// layout while vw units ignore zoom, so dialogs/toasts spill sideways and the
// page can be dragged horizontally. Students open the race page on phones, so
// that breakpoint must stay pixel-exact.
const BASE_SCALE = 1.25 // v2.1 design baseline: 100% in settings equals this
const NARROW_VIEWPORT = window.matchMedia('(max-width: 760px)')
let patched = false
let currentPercent = 100

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

function effectiveScale(percent: number): number {
  return NARROW_VIEWPORT.matches ? 1 : (percent / 100) * BASE_SCALE
}

export function applyInterfaceScale(percent: number) {
  installRectPatch()
  currentPercent = percent
  document.documentElement.style.zoom = String(effectiveScale(percent))
}

// Rotating a phone or shrinking a laptop window crosses the breakpoint; keep
// the zoom in sync without a reload.
NARROW_VIEWPORT.addEventListener('change', () => {
  if (patched) document.documentElement.style.zoom = String(effectiveScale(currentPercent))
})
