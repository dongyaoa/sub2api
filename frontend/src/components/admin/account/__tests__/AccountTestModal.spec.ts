import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountTestModal from '../AccountTestModal.vue'

const { getAvailableModels, copyToClipboard } = vi.hoisted(() => ({
  getAvailableModels: vi.fn(),
  copyToClipboard: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getAvailableModels
    }
  }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'admin.accounts.imagePromptDefault': 'Generate a cute orange cat astronaut sticker on a clean pastel background.'
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.accounts.imageReceived' && params?.count) {
          return `received-${params.count}`
        }
        if (key === 'admin.accounts.imagePreviewAlt' && params?.index) {
          return `test-image-${params.index}`
        }
        if (key === 'admin.accounts.testProxySelected') {
          return `Test proxy: ${params?.name} (ID: ${params?.id})`
        }
        return messages[key] || key
      }
    })
  }
})

function createStreamResponse(lines: string[]) {
  const encoder = new TextEncoder()
  const chunks = lines.map((line) => encoder.encode(line))
  let index = 0

  return {
    ok: true,
    body: {
      getReader: () => ({
        read: vi.fn().mockImplementation(async () => {
          if (index < chunks.length) {
            return { done: false, value: chunks[index++] }
          }
          return { done: true, value: undefined }
        })
      })
    }
  } as Response
}

function mountModal(account: Record<string, unknown> = {
  id: 42,
  name: 'Gemini Image Test',
  platform: 'gemini',
  type: 'apikey',
  status: 'active'
}) {
  return mount(AccountTestModal, {
    props: {
      show: false,
      account
    } as any,
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Select: {
          props: ['modelValue', 'options', 'disabled', 'valueKey', 'labelKey'],
          emits: ['update:modelValue'],
          template: `<select
            class="select-stub"
            :value="modelValue ?? ''"
            :disabled="disabled"
            @change="$emit('update:modelValue', options.find(option => String(option[valueKey || 'value'] ?? '') === $event.target.value)?.[valueKey || 'value'] ?? null)"
          >
            <option v-for="(option, index) in options" :key="index" :value="option[valueKey || 'value'] ?? ''" :disabled="option.disabled">
              {{ option[labelKey || 'label'] }}
            </option>
          </select>`
        },
        HelpTooltip: { props: ['content'], template: '<span :title="content" />' },
        TextArea: {
          props: ['modelValue'],
          emits: ['update:modelValue'],
          template: '<textarea class="textarea-stub" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
        },
        Icon: true
      }
    }
  })
}

describe('AccountTestModal', () => {
  beforeEach(() => {
    getAvailableModels.mockResolvedValue([
      { id: 'gemini-2.0-flash', display_name: 'Gemini 2.0 Flash' },
      { id: 'gemini-2.5-flash-image', display_name: 'Gemini 2.5 Flash Image' },
      { id: 'gemini-3.1-flash-image', display_name: 'Gemini 3.1 Flash Image' }
    ])
    copyToClipboard.mockReset()
    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: vi.fn((key: string) => (key === 'auth_token' ? 'test-token' : null)),
        setItem: vi.fn(),
        removeItem: vi.fn(),
        clear: vi.fn()
      },
      configurable: true
    })
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"gemini-2.5-flash-image"}\n',
        'data: {"type":"image","image_url":"data:image/png;base64,QUJD","mime_type":"image/png"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('gemini 图片模型测试会携带提示词并渲染图片预览', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const promptInput = wrapper.find('textarea.textarea-stub')
    expect(promptInput.exists()).toBe(true)
    await promptInput.setValue('draw a tiny orange cat astronaut')

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'gemini-3.1-flash-image',
      prompt: 'draw a tiny orange cat astronaut'
    })

    const preview = wrapper.find('img[alt="test-image-1"]')
    expect(preview.exists()).toBe(true)
    expect(preview.attributes('src')).toBe('data:image/png;base64,QUJD')
  })

  it('grok 账号测试默认选择 Grok 模型', async () => {
    getAvailableModels.mockResolvedValue([
      { id: 'grok-4.3', display_name: 'Grok 4.3' },
      { id: 'grok-build-0.1', display_name: 'Grok Build 0.1' }
    ])
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"grok-4.3"}\n',
        'data: {"type":"content","text":"ok"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      id: 13,
      name: 'Grok Account',
      platform: 'grok',
      type: 'oauth',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'grok-4.3',
      prompt: '',
      mode: 'text'
    })
  })

  it('OpenAI Compact 探测会携带 compact 测试模式', async () => {
    getAvailableModels.mockResolvedValue([
      { id: 'gpt-5.4', display_name: 'GPT-5.4' }
    ])
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      id: 42,
      name: 'OpenAI OAuth',
      platform: 'openai',
      type: 'oauth',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    ;(wrapper.vm as any).selectedModelId = 'gpt-5.4'
    ;(wrapper.vm as any).testMode = 'compact'
    await (wrapper.vm as any).startTest()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toMatchObject({
      model_id: 'gpt-5.4',
      prompt: '',
      mode: 'compact'
    })
  })

  it('显示服务端实际选中的代理并在重试等待期间清除旧代理', async () => {
    let finishRetry!: (response: Response) => void
    global.fetch = vi.fn()
      .mockResolvedValueOnce(createStreamResponse([
        'data: {"type":"proxy_info","route_type":"managed","proxy_id":12,"proxy_name":"US selected node"}\n',
        'data: {"type":"error","error":"API returned 429"}\n'
      ]))
      .mockImplementationOnce(() => new Promise<Response>((resolve) => { finishRetry = resolve })) as any

    const wrapper = mountModal({
      id: 42,
      name: 'Multiple proxy account',
      platform: 'gemini',
      type: 'apikey',
      status: 'active',
      proxy_id: 11,
      proxy: { id: 11, name: 'Legacy primary node' }
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    const startButton = wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.startTest'))
    await startButton!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('US selected node')
    expect(wrapper.text()).toContain('ID: 12')
    expect(wrapper.get('[data-testid="account-test-actual-proxy"]').text()).not.toContain('Legacy primary node')
    expect(wrapper.text()).toContain('API returned 429')
    const copyButton = wrapper.find('button[title="admin.accounts.copyOutput"]')
    await copyButton.trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith(
      expect.stringContaining('Test proxy: US selected node (ID: 12)'),
      'admin.accounts.outputCopied'
    )

    const retryButton = wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.retry'))
    await retryButton!.trigger('click')
    await flushPromises()
    expect(global.fetch).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).not.toContain('US selected node')
    expect(wrapper.text()).not.toContain('ID: 12')

    finishRetry(createStreamResponse([
      'data: {"type":"proxy_info","route_type":"managed","proxy_id":13,"proxy_name":"EU retry node"}\n',
      'data: {"type":"test_complete","success":true}\n'
    ]))
    await flushPromises()
    expect(wrapper.text()).toContain('EU retry node')
    expect(wrapper.text()).toContain('ID: 13')
    expect(wrapper.text()).not.toContain('US selected node')
    wrapper.unmount()
  })

  it.each(['direct', 'unknown'])('显示 %s 路由且重新打开时清除旧结果', async (routeType) => {
    global.fetch = vi.fn().mockResolvedValue(createStreamResponse([
      `data: {"type":"proxy_info","route_type":"${routeType}"}\n`,
      'data: {"type":"test_complete","success":true}\n'
    ])) as any
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()
    const startButton = wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.startTest'))
    await startButton!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain(`admin.accounts.testProxyRoute.${routeType}`)
    expect(wrapper.text()).not.toContain('ID:')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.text()).not.toContain(`admin.accounts.testProxyRoute.${routeType}`)
    wrapper.unmount()
  })

  it('仅提供绑定代理并禁用停用、过期和未加载的代理，指定选择后携带代理 ID', async () => {
    const wrapper = mountModal({
      id: 42,
      name: 'Proxy pool account',
      platform: 'gemini',
      type: 'apikey',
      status: 'active',
      proxy_id: 99,
      proxy: { id: 99, name: 'Outside pool', status: 'active' },
      proxy_pool: [
        { proxy_id: 12, concurrency: 1, proxy: { id: 12, name: 'US node', status: 'active' } },
        { proxy_id: 13, concurrency: 1, proxy: { id: 13, name: 'Disabled node', status: 'inactive' } },
        { proxy_id: 14, concurrency: 1, proxy: { id: 14, name: 'Expired node', status: 'expired' } },
        { proxy_id: 15, concurrency: 1 },
        { proxy_id: 16, concurrency: 1, proxy: { id: 16, name: 'Past expiry', status: 'active', expires_at: '2000-01-01T00:00:00Z' } }
      ]
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    const proxySelect = wrapper.get('[data-testid="account-test-proxy-select"]')
    expect(proxySelect.findAll('option').map(option => option.element.value)).toEqual(['', '12', '13', '14', '15', '16'])
    expect(proxySelect.text()).not.toContain('Outside pool')
    expect(proxySelect.get('option[value="12"]').attributes('disabled')).toBeUndefined()
    for (const id of [13, 14, 15, 16]) {
      expect(proxySelect.get(`option[value="${id}"]`).attributes('disabled')).toBeDefined()
    }
    expect(proxySelect.get('option[value="13"]').text()).toContain('admin.accounts.testProxyOptions.inactive')
    expect(proxySelect.get('option[value="14"]').text()).toContain('admin.accounts.testProxyOptions.expired')
    expect(proxySelect.get('option[value="15"]').text()).toContain('admin.accounts.testProxyOptions.unavailable')
    await proxySelect.setValue('12')
    await wrapper.findAll('button').find(button => button.text().includes('admin.accounts.startTest'))!.trigger('click')
    await flushPromises()

    expect(JSON.parse(vi.mocked(global.fetch).mock.calls[0][1]!.body as string).proxy_id).toBe(12)
    wrapper.unmount()
  })

  it('兼容旧版单代理绑定且自动选择不提交代理 ID', async () => {
    const wrapper = mountModal({
      id: 42,
      name: 'Legacy proxy account',
      platform: 'gemini',
      type: 'apikey',
      status: 'active',
      proxy_id: 11,
      proxy: { id: 11, name: 'Legacy node', status: 'active' }
    })
    await wrapper.setProps({ show: true })
    await flushPromises()
    const proxySelect = wrapper.get('[data-testid="account-test-proxy-select"]')
    expect(proxySelect.findAll('option').map(option => option.element.value)).toEqual(['', '11'])
    expect((proxySelect.element as HTMLSelectElement).value).toBe('')
    await wrapper.findAll('button').find(button => button.text().includes('admin.accounts.startTest'))!.trigger('click')
    await flushPromises()
    expect(JSON.parse(vi.mocked(global.fetch).mock.calls[0][1]!.body as string)).not.toHaveProperty('proxy_id')
    wrapper.unmount()
  })

  it('重试保留代理选择并清除旧耗时，连接时禁用选择，重新打开恢复自动选择', async () => {
    let finishRetry!: (response: Response) => void
    global.fetch = vi.fn()
      .mockResolvedValueOnce(createStreamResponse([
        'data: {"type":"test_metrics","latency_ms":120,"first_token_ms":280,"duration_ms":930}\n',
        'data: {"type":"error","error":"API returned 429"}\n'
      ]))
      .mockImplementationOnce(() => new Promise<Response>(resolve => { finishRetry = resolve })) as any
    const wrapper = mountModal({
      id: 42,
      name: 'Retry account',
      platform: 'gemini',
      type: 'apikey',
      status: 'active',
      proxy_pool: [{ proxy_id: 12, concurrency: 1, proxy: { id: 12, name: 'US node', status: 'active' } }]
    })
    await wrapper.setProps({ show: true })
    await flushPromises()
    const proxySelect = wrapper.get('[data-testid="account-test-proxy-select"]')
    await proxySelect.setValue('12')
    await wrapper.findAll('button').find(button => button.text().includes('admin.accounts.startTest'))!.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="account-test-metric-latency_ms"]').text()).toBe('120 ms')
    expect(wrapper.get('[data-testid="account-test-metric-first_token_ms"]').text()).toBe('280 ms')
    expect(wrapper.get('[data-testid="account-test-metric-duration_ms"]').text()).toBe('930 ms')

    await wrapper.findAll('button').find(button => button.text().includes('admin.accounts.retry'))!.trigger('click')
    await flushPromises()
    expect((proxySelect.element as HTMLSelectElement).value).toBe('12')
    expect(proxySelect.attributes('disabled')).toBeDefined()
    expect(JSON.parse(vi.mocked(global.fetch).mock.calls[1][1]!.body as string).proxy_id).toBe(12)
    expect(wrapper.findAll('[data-testid^="account-test-metric-"]').map(metric => metric.text())).toEqual(['--', '--', '--'])

    finishRetry(createStreamResponse([
      'data: {"type":"test_metrics","duration_ms":450}\n',
      'data: {"type":"test_complete","success":true}\n'
    ]))
    await flushPromises()
    expect(proxySelect.attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-testid="account-test-metric-duration_ms"]').text()).toBe('450 ms')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect((proxySelect.element as HTMLSelectElement).value).toBe('')
    expect(wrapper.find('[data-testid="account-test-metrics"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('分批接收耗时保留有效零值，缺失项显示占位且复制输出包含耗时', async () => {
    global.fetch = vi.fn().mockResolvedValue(createStreamResponse([
      'data: {"type":"test_metrics","latency_ms":0}\n',
      'data: {"type":"test_metrics","duration_ms":375}\n',
      'data: {"type":"test_complete","success":true}\n'
    ])) as any
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('admin.accounts.startTest'))!.trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="account-test-metric-latency_ms"]').text()).toBe('0 ms')
    expect(wrapper.get('[data-testid="account-test-metric-first_token_ms"]').text()).toBe('--')
    expect(wrapper.get('[data-testid="account-test-metric-duration_ms"]').text()).toBe('375 ms')
    await wrapper.get('button[title="admin.accounts.copyOutput"]').trigger('click')
    const copiedOutput = copyToClipboard.mock.calls[0][0] as string
    expect(copiedOutput).toContain('admin.accounts.testMetrics.latency: 0 ms')
    expect(copiedOutput).toContain('admin.accounts.testMetrics.firstToken: --')
    expect(copiedOutput).toContain('admin.accounts.testMetrics.duration: 375 ms')
    wrapper.unmount()
  })

  it('非法或空的耗时不会显示为零毫秒', async () => {
    global.fetch = vi.fn().mockResolvedValue(createStreamResponse([
      'data: {"type":"test_metrics","latency_ms":-1,"first_token_ms":null,"duration_ms":"150"}\n',
      'data: {"type":"test_complete","success":true}\n'
    ])) as any
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('admin.accounts.startTest'))!.trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid^="account-test-metric-"]').map(metric => metric.text())).toEqual(['--', '--', '--'])
    wrapper.unmount()
  })
})
