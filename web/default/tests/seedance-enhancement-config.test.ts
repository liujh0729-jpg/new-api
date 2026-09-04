import { describe, expect, test } from 'bun:test'
import {
  buildMediaKitSpecification,
  seedanceProviderModeLabel,
  seedanceProviderOptionLabel,
} from '../src/features/seedance-admin/lib/enhancement-config'

describe('Seedance enhancement configuration', () => {
  test('builds the fixed MediaKit specification shape', () => {
    expect(
      JSON.parse(buildMediaKitSpecification('1080p', 'professional'))
    ).toEqual({
      scene: 'aigc',
      resolution: '1080p',
      tool_version: 'professional',
    })
  })

  test('uses Chinese adapter and status labels', () => {
    expect(seedanceProviderModeLabel('VOLCENGINE_MEDIAKIT')).toBe(
      '火山 AI MediaKit'
    )
    expect(
      seedanceProviderOptionLabel({
        id: 1,
        version: 1,
        provider_type: 'DIRECT_EXTERNAL',
        adapter_type: 'GENERIC_HTTP',
        display_name: '远端节点',
        service_endpoint: 'https://example.com/tasks',
        service_code: 'video_sr',
        capabilities: '{}',
        credential_configured: false,
        status: 'DISABLED',
        timeout_policy: '{}',
        retry_policy: '{}',
        fallback_policy: '{}',
        created_at: 0,
        updated_at: 0,
      })
    ).toBe('远端节点 · 自定义远端服务 · 停用')
  })
})
