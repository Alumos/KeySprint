// navigator.clipboard 只在安全上下文（HTTPS 或 localhost）可用；机房局域网是
// http://<教师机IP>:8080，属于非安全上下文，因此退回 legacy execCommand 方案，
// 保证「复制比赛编号」在局域网部署下同样可用。
export async function copyText(text: string) {
  if (navigator.clipboard?.writeText) {
    return navigator.clipboard.writeText(text)
  }
  const area = document.createElement('textarea')
  area.value = text
  area.setAttribute('readonly', '')
  area.style.position = 'fixed'
  area.style.top = '-1000px'
  area.style.opacity = '0'
  document.body.appendChild(area)
  area.focus()
  area.select()
  area.setSelectionRange(0, text.length)
  const ok = document.execCommand('copy')
  area.remove()
  if (!ok) throw new Error('当前浏览器不支持自动复制，请手动选择比赛编号')
}
