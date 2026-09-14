/**
 * 把后端 / 未知异常统一转成可以直接展示给用户的错误文案。
 *
 * Wails 绑定在不同场景下会抛出 Error、字符串或普通对象，
 * 统一在这里兜底，避免调用方只能显示一句笼统的「操作失败」。
 */
export function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error) {
    const message = error.message?.trim()
    if (message) return message
  }
  if (typeof error === 'string') {
    const message = error.trim()
    if (message) return message
  }
  return fallback
}

export default errorMessage
