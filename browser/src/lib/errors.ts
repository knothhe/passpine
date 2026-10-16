const messages: Record<string, string> = {
  app_unavailable: '请先打开 Passpine 客户端，再重新连接。',
  native_connection_failed: '尚未连接客户端。请在客户端的「浏览器连接」中安装 Native Messaging Host。',
  background_unavailable: '扩展后台未响应，请重新打开扩展或在扩展管理页重新加载。',
  invalid_response: '客户端返回了无法识别的响应，请重新连接客户端。',
  locked: '密码库已锁定。请在客户端解锁后重试。',
  system_unlock_required: '请先在客户端使用私钥口令解锁，并启用系统解锁。',
  authentication_cancelled: '系统验证已取消或未完成，密码库仍保持锁定。',
  unauthorized: '浏览器扩展尚未获得客户端授权，请检查扩展 ID。',
  protocol_mismatch: '客户端与扩展的协议版本不同，请同时更新两端。',
  vault_changed: '当前密码库已切换，请刷新列表后重试。',
  revision_conflict: '此条目已在其他位置修改，请重新打开后编辑。',
  revision_required: '请重新打开此条目后再修改。',
  transport_error: '与客户端的连接中断。此操作不会自动重试，请检查客户端。',
  tab_changed: '页面已经变化，请重新打开扩展后再填充。',
  domain_mismatch: '此账号尚未关联当前网站，请先确认并关联网站。',
  no_fields: '当前页面没有可填写的登录表单。',
  invalid_request: '请求格式不正确，请刷新后重试。',
};
export class PassPineError extends Error {
  constructor(public code: string, message?: string) { super(messages[code] ?? message ?? '操作未完成，请重试。'); }
}
export function errorMessage(error: unknown): string { return error instanceof Error ? error.message : '操作未完成，请重试。'; }
