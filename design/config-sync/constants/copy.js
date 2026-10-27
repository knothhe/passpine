window.configSyncCopy = {
  title: '配置同步', description: '将个人偏好加密保存到你自己的对象存储，在其他设备恢复。',
  settings: '设置', general: '通用', security: '安全', vault: '密码库', browser: '浏览器', cli: '命令行', updates: '更新', about: '关于',
  method: '连接方式', oauth: 'Cloudflare 授权', manual: '手动配置 S3',
  oauthHelp: '在浏览器中授权，然后选择自己的 R2 存储桶。', manualHelp: '支持 Cloudflare R2 和其他兼容 S3 的对象存储。',
  connect: '连接 Cloudflare', cancel: '取消', waiting: '等待浏览器授权', waitingHelp: '请在 Cloudflare 同意页检查权限。若 Account API Tokens Write 未启用，请点击 Edit Permissions 开启。',
  account: '账号', jurisdiction: '辖区', bucket: '存储桶', defaultJurisdiction: '默认',
  endpoint: '服务端点', region: '区域', prefix: '目录前缀', key: 'Access Key ID', secret: 'Secret Access Key', session: 'Session Token（可选）', pathStyle: '使用路径寻址',
  phrase: '同步加密口令', phraseHelp: '在其他设备输入相同口令以读取配置。口令仅保存在本机系统凭据存储。',
  prefixHelp: '其他设备需使用相同的存储桶、前缀和加密口令。',
  test: '测试连接', save: '保存并连接', select: '使用此存储桶', saved: '已连接', lastSync: '上次同步：刚刚', sync: '立即同步', edit: '编辑连接', disconnect: '断开连接',
  scope: '同步内容', scopeHelp: '客户端与扩展分别同步主题、语言和字号。密码库仍使用原有 Git 同步。',
  localHelp: '本机路径、私钥、系统解锁、开机自启和浏览器权限保留在本机。',
  error: '连接未保存', errorHelp: '无法访问所选存储桶。请检查地址与访问权限，然后重试；已填写内容会保留。',
  conflict: '部分配置有冲突', conflictHelp: '此设备与其他设备修改了相同设置。本次同步未覆盖任何一端，请选择要保留的配置后重试。',
  keepLocal: '保留此设备配置', useRemote: '使用云端配置', conflictField: '客户端主题',
  disconnectedBoard: '选择连接方式', manualBoard: '手动配置 S3', waitingBoard: '等待授权', bucketBoard: '选择账号与存储桶', connectedBoard: '已连接', errorBoard: '连接失败与重试', conflictBoard: '冲突处理',
  canvasNote: '设置使用独立同步空间；编辑连接不会立即切换同步目标，保存成功后生效。所有示例均为虚构数据。'
};
