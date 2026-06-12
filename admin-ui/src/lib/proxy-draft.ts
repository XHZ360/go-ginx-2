// proxy-draft：HTTPS proxy 表单跳转创建证书时的草稿与导航契约。
//
// 流程：
//  1. 用户在 proxy 创建/编辑表单点击「创建证书」。proxy 表单调用 saveProxyDraft(data)
//     把当前表单快照（不含任何 secret material）写入 sessionStorage，得到 draftId。
//  2. 表单导航到 buildCertificateCreateLink({ returnTo, draftId, host })，进入 Certificates 页
//     的证书创建流程（通过 ?create=1 query param 打开创建对话框）。
//  3. 证书创建成功后，Certificates 页调用 appendCreatedCertificate(returnTo, draftId, certId)
//     回到 returnTo，并在 query 中带上 createdCertificateId + draftId。
//  4. proxy 表单读取 draftId 还原草稿（loadProxyDraft），并用 createdCertificateId 自动选中新证书。
//  5. 还原成功或失败后调用 clearProxyDraft(draftId) 清理。
//
// 草稿数据形状由 proxy 表单（生产者/消费者）定义，本模块对其内容保持透明（泛型 T）。
// Certificates 页只透传 returnTo / draftId / createdCertificateId，无需理解草稿内容。

const DRAFT_KEY_PREFIX = 'goginx.admin.proxyDraft.';
const DRAFT_TTL_MS = 30 * 60 * 1000; // 30 分钟，超时按缺失处理（安全降级）

// 统一的 query param 名称，两端必须一致。
export const CREATE_PARAM = 'create';
export const RETURN_TO_PARAM = 'returnTo';
export const DRAFT_ID_PARAM = 'draftId';
export const CREATED_CERT_PARAM = 'createdCertificateId';
export const HOST_HINT_PARAM = 'host';
export const PROVIDER_HINT_PARAM = 'providerType';

// 允许作为 returnTo 的安全路由，防止开放重定向。
const SAFE_RETURN_ROUTES = [/^\/proxies$/, /^\/proxies\/[^/]+$/];

interface DraftEnvelope<T> {
  savedAt: number;
  data: T;
}

function storage(): Storage | null {
  if (typeof window === 'undefined') {
    return null;
  }
  try {
    return window.sessionStorage;
  } catch {
    return null;
  }
}

function draftKey(draftId: string): string {
  return `${DRAFT_KEY_PREFIX}${draftId}`;
}

function newDraftId(): string {
  const cryptoObj = typeof globalThis !== 'undefined' ? globalThis.crypto : undefined;
  if (cryptoObj && typeof cryptoObj.randomUUID === 'function') {
    return cryptoObj.randomUUID();
  }
  // 退化方案：时间戳 + 随机数，足以避免同源短期碰撞。
  return `d${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

// saveProxyDraft 持久化一份草稿快照并返回 draftId。
// 调用方负责确保 data 中不包含私钥、token 等 secret material。
export function saveProxyDraft<T>(data: T): string {
  const store = storage();
  const draftId = newDraftId();
  if (!store) {
    return draftId;
  }
  const envelope: DraftEnvelope<T> = { savedAt: Date.now(), data };
  try {
    store.setItem(draftKey(draftId), JSON.stringify(envelope));
  } catch {
    // 存储失败时仍返回 draftId；消费端会在草稿缺失时安全降级。
  }
  return draftId;
}

// loadProxyDraft 读取草稿。草稿缺失、过期或解析失败时返回 null（消费端据此安全降级）。
export function loadProxyDraft<T>(draftId: string | null | undefined): T | null {
  if (!draftId) {
    return null;
  }
  const store = storage();
  if (!store) {
    return null;
  }
  const raw = store.getItem(draftKey(draftId));
  if (!raw) {
    return null;
  }
  try {
    const envelope = JSON.parse(raw) as DraftEnvelope<T>;
    if (!envelope || typeof envelope.savedAt !== 'number') {
      return null;
    }
    if (Date.now() - envelope.savedAt > DRAFT_TTL_MS) {
      store.removeItem(draftKey(draftId));
      return null;
    }
    return envelope.data;
  } catch {
    return null;
  }
}

// clearProxyDraft 删除草稿。还原成功或放弃后调用。
export function clearProxyDraft(draftId: string | null | undefined): void {
  if (!draftId) {
    return;
  }
  storage()?.removeItem(draftKey(draftId));
}

// sanitizeReturnTo 仅放行已知的 proxy 路由（含 query/hash），其余回退到 /proxies。
export function sanitizeReturnTo(returnTo?: string | null): string {
  if (!returnTo) {
    return '/proxies';
  }
  let parsed: URL;
  try {
    parsed = new URL(returnTo, 'http://admin.local');
  } catch {
    return '/proxies';
  }
  if (parsed.origin !== 'http://admin.local') {
    return '/proxies';
  }
  if (!SAFE_RETURN_ROUTES.some((pattern) => pattern.test(parsed.pathname))) {
    return '/proxies';
  }
  return `${parsed.pathname}${parsed.search}${parsed.hash}`;
}

// buildCertificateCreateLink 构造跳转到 Certificates 页证书创建流程的链接。
export function buildCertificateCreateLink(opts: {
  returnTo: string;
  draftId: string;
  host?: string;
  providerType?: string;
}): string {
  const params = new URLSearchParams();
  params.set(CREATE_PARAM, '1');
  params.set(RETURN_TO_PARAM, sanitizeReturnTo(opts.returnTo));
  params.set(DRAFT_ID_PARAM, opts.draftId);
  if (opts.host) {
    params.set(HOST_HINT_PARAM, opts.host);
  }
  if (opts.providerType) {
    params.set(PROVIDER_HINT_PARAM, opts.providerType);
  }
  return `/certificates?${params.toString()}`;
}

// appendCreatedCertificate 在 returnTo 基础上附加 createdCertificateId + draftId，
// 供 Certificates 页在证书创建成功后导航回 proxy 表单使用。
export function appendCreatedCertificate(
  returnTo: string,
  draftId: string,
  certificateId: string,
): string {
  const safe = sanitizeReturnTo(returnTo);
  const url = new URL(safe, 'http://admin.local');
  url.searchParams.set(CREATED_CERT_PARAM, certificateId);
  if (draftId) {
    url.searchParams.set(DRAFT_ID_PARAM, draftId);
  }
  return `${url.pathname}${url.search}${url.hash}`;
}
