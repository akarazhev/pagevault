export const DOWNLOADS_URL = 'https://github.com/akarazhev/pagevault/releases';

function versionParts(value) {
  if (typeof value !== 'string' || !/^\d+\.\d+\.\d+(?:\.\d+)?$/.test(value)) return null;
  const parts = value.split('.').map(Number);
  return parts.every(Number.isSafeInteger) ? parts : null;
}
function isOlder(current, expected) {
  const a = versionParts(current), b = versionParts(expected);
  if (!a || !b) return false;
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    if ((a[i] || 0) !== (b[i] || 0)) return (a[i] || 0) < (b[i] || 0);
  }
  return false;
}
export function connectionState(host, extensionVersion) {
  const version = typeof host?.version === 'string' ? host.version : 'Неизвестна';
  const error = typeof host?.error === 'string' ? host.error : '';
  const dbPath = typeof host?.db_path === 'string' ? host.db_path : '';
  const base = {version, dbPath, error, usable:false};
  if (!host?.connected) return {...base, dbPath:'', kind:'unavailable', label:'Нет связи с локальным помощником', detail:'Установите или обновите помощника для вашего браузера, затем проверьте подключение. Если он уже установлен, посмотрите ошибку ниже.'};
  if (host.schema_version != null && host.schema_version !== 1) return {...base, kind:'incompatible', label:'Помощник несовместим', detail:'Помощник использует неподдерживаемую схему архива. Установите совместимую версию перед работой с этой базой.'};
  const update = isOlder(host.version, extensionVersion);
  return {...base, usable:true, kind:update ? 'update' : 'ready', label:`SQLite подключена · помощник ${version}`, detail:update ? 'Подключение работает. Установлена более ранняя версия помощника; после обновления переподключите браузер и MCP-клиент.' : 'Помощник подключён. Браузер запускает его автоматически; отдельный сервер запускать не нужно.'};
}
export function normalizePlatform(value) {
  return ['mac','win','linux'].includes(value) ? value : 'unknown';
}
export function platformInstructions(platform) {
  switch (normalizePlatform(platform)) {
    case 'mac': return {detail:'macOS 13 и новее: откройте загрузки и выберите пакет .pkg. Он подходит для Intel и Apple Silicon.', install:'Откройте .pkg и выберите используемые браузеры. Установка выполняется для текущего пользователя и сохраняет путь существующего архива.', command:''};
    case 'win': return {detail:'Windows: скачайте и распакуйте полный комплект PageVault с готовыми программами.', install:'Откройте install-windows.cmd двойным щелчком для Chrome. Для Edge, Brave или Chromium используйте инструкции по ссылке ниже.', command:'.\\install-windows.cmd'};
    case 'linux': return {detail:'Linux: скачайте и распакуйте полный комплект PageVault с готовыми программами.', install:'В терминале из папки комплекта выполните команду для Chrome. Для Edge, Brave или Chromium замените chrome на нужный браузер.', command:'bash install.sh chrome'};
    default: return {detail:'Выберите вашу операционную систему. На странице загрузок доступны опубликованные выпуски PageVault.', install:'После выбора системы здесь появятся инструкции установки локального компонента.', command:''};
  }
}
