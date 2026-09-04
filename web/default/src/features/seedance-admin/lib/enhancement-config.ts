import type { SeedanceProvider } from '../types'

export const MEDIAKIT_BASE_URL = 'https://mediakit.cn-beijing.volces.com'
export const MEDIAKIT_SERVICE_CODE = 'volcengine_ai_mediakit_quality_enhance'

export type MediaKitResolution = '480p' | '720p' | '1080p' | '2k' | '4k'
export type MediaKitToolVersion = 'standard' | 'professional'

export function seedanceProviderModeLabel(
  adapterType: SeedanceProvider['adapter_type']
) {
  return adapterType === 'VOLCENGINE_MEDIAKIT'
    ? '火山 AI MediaKit'
    : '自定义远端服务'
}

export function buildMediaKitSpecification(
  resolution: MediaKitResolution,
  toolVersion: MediaKitToolVersion
) {
  return JSON.stringify({
    scene: 'aigc',
    resolution,
    tool_version: toolVersion,
  })
}

export function seedanceProviderOptionLabel(provider: SeedanceProvider) {
  const status = provider.status === 'ACTIVE' ? '启用' : '停用'
  return `${provider.display_name} · ${seedanceProviderModeLabel(provider.adapter_type)} · ${status}`
}
