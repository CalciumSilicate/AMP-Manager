import { listAvailableModels, type AvailableModel } from '@/api/models'

export type CCSwitchApp = 'codex' | 'opencode' | 'openclaw'

const CODEX_PROVIDER_NAME = 'OpenAI'

export interface APIKeyUsageContent {
  defaultModel: string
  models: AvailableModel[]
  ccSwitchLinks: Record<CCSwitchApp, string>
  ccSwitchFallbackPatch: string
  codex: {
    configToml: string
    authJson: string
    command: string
  }
  codexWebsocket: {
    configToml: string
    authJson: string
    command: string
  }
  opencode: {
    configJson: string
    command: string
  }
  openclaw: {
    configJson: string
    command: string
  }
}

export async function getAPIKeyAvailableModels({
  signal,
}: {
  signal?: AbortSignal
}): Promise<AvailableModel[]> {
  if (signal?.aborted) {
    throw new DOMException('Aborted', 'AbortError')
  }

  const models = await listAvailableModels()
  const deduped = new Map<string, AvailableModel>()

  for (const item of models) {
    const modelId = item.modelId?.trim()
    if (!modelId) continue
    if (!deduped.has(modelId)) {
      deduped.set(modelId, item)
    }
  }

  if (deduped.size === 0) {
    throw new Error('获取模型列表失败')
  }

  return Array.from(deduped.values())
}

export function buildAPIKeyUsageContent({
  origin,
  apiBaseUrl,
  apiKey,
  keyName,
  models,
  selectedModelId,
}: {
  origin: string
  apiBaseUrl: string
  apiKey: string
  keyName: string
  models: AvailableModel[]
  selectedModelId: string
}): APIKeyUsageContent {
  const selectedModel = models.find((item) => item.modelId === selectedModelId) ?? models[0]
  const defaultModel = selectedModel.modelId

  return {
    defaultModel,
    models,
    ccSwitchLinks: {
      codex: buildCCSwitchDeepLink({
        app: 'codex',
        origin,
        apiBaseUrl,
        apiKey,
        keyName,
        defaultModel,
      }),
      opencode: buildCCSwitchDeepLink({
        app: 'opencode',
        origin,
        apiBaseUrl,
        apiKey,
        keyName,
        defaultModel,
      }),
      openclaw: buildCCSwitchDeepLink({
        app: 'openclaw',
        origin,
        apiBaseUrl,
        apiKey,
        keyName,
        defaultModel,
      }),
    },
    ccSwitchFallbackPatch: buildCCSwitchFallbackPatch(defaultModel),
    codex: {
      configToml: buildCodexConfigToml(apiBaseUrl, selectedModel, false),
      authJson: buildCodexAuthJson(apiKey),
      command: 'codex',
    },
    codexWebsocket: {
      configToml: buildCodexConfigToml(apiBaseUrl, selectedModel, true),
      authJson: buildCodexAuthJson(apiKey),
      command: 'codex',
    },
    opencode: {
      configJson: buildOpencodeConfig(apiBaseUrl, apiKey, models),
      command: `opencode -m openai/${defaultModel}`,
    },
    openclaw: {
      configJson: buildOpenclawConfig(apiBaseUrl, apiKey, models),
      command: 'openclaw',
    },
  }
}

function buildCCSwitchDeepLink({
  app,
  origin,
  apiBaseUrl,
  apiKey,
  keyName,
  defaultModel,
}: {
  app: CCSwitchApp
  origin: string
  apiBaseUrl: string
  apiKey: string
  keyName: string
  defaultModel: string
}): string {
  const params = new URLSearchParams({
    resource: 'provider',
    app,
    name: `AMP Manager (${keyName})`,
    homepage: origin,
    endpoint: apiBaseUrl,
    apiKey,
    model: defaultModel,
    enabled: 'true',
  })

  return `ccswitch://v1/import?${params.toString()}`
}

function buildCCSwitchFallbackPatch(defaultModel: string): string {
  return JSON.stringify(
    {
      meta: {
        testConfig: {
          enabled: true,
          testModel: defaultModel,
        },
      },
    },
    null,
    2,
  )
}

function buildCodexConfigToml(apiBaseUrl: string, selectedModel: AvailableModel, websocket: boolean): string {
  const lines = [
    `model_provider = "${CODEX_PROVIDER_NAME}"`,
    `model = "${selectedModel.modelId}"`,
    `review_model = "${selectedModel.modelId}"`,
    'model_reasoning_effort = "xhigh"',
    'disable_response_storage = true',
    'network_access = "enabled"',
    'windows_wsl_setup_acknowledged = true',
  ]

  if (selectedModel.contextLength && selectedModel.contextLength > 0) {
    lines.push(`model_context_window = ${selectedModel.contextLength}`)
    lines.push(`model_auto_compact_token_limit = ${Math.floor(selectedModel.contextLength * 0.9)}`)
  }

  lines.push('')
  lines.push(`[model_providers.${CODEX_PROVIDER_NAME}]`)
  lines.push(`name = "${CODEX_PROVIDER_NAME}"`)
  lines.push(`base_url = "${apiBaseUrl}"`)
  lines.push('wire_api = "responses"')
  lines.push('requires_openai_auth = true')
  if (websocket) {
    lines.push('supports_websockets = true')
  }

  const featureSection = websocket
    ? `

[features]
responses_websockets_v2 = true`
    : ''

  return `${lines.join('\n')}${featureSection}\n`
}

function buildCodexAuthJson(apiKey: string): string {
  return JSON.stringify(
    {
      OPENAI_API_KEY: apiKey,
    },
    null,
    2,
  )
}

function buildOpencodeConfig(apiBaseUrl: string, apiKey: string, models: AvailableModel[]): string {
  const modelEntries = Object.fromEntries(
    models.map((modelItem) => {
      const modelConfig: Record<string, unknown> = {
        name: modelItem.modelId,
        options: {
          store: false,
        },
        variants: {
          low: {},
          medium: {},
          high: {},
          xhigh: {},
        },
      }

      if ((modelItem.contextLength && modelItem.contextLength > 0) || (modelItem.maxCompletionTokens && modelItem.maxCompletionTokens > 0)) {
        modelConfig.limit = {
          ...(modelItem.contextLength && modelItem.contextLength > 0 ? { context: modelItem.contextLength } : {}),
          ...(modelItem.maxCompletionTokens && modelItem.maxCompletionTokens > 0 ? { output: modelItem.maxCompletionTokens } : {}),
        }
      }

      return [modelItem.modelId, modelConfig]
    }),
  )

  return JSON.stringify(
    {
      provider: {
        openai: {
          options: {
            baseURL: apiBaseUrl,
            apiKey,
          },
          models: modelEntries,
        },
      },
      agent: {
        build: {
          options: {
            store: false,
          },
        },
        plan: {
          options: {
            store: false,
          },
        },
      },
      $schema: 'https://opencode.ai/config.json',
    },
    null,
    2,
  )
}

function buildOpenclawConfig(apiBaseUrl: string, apiKey: string, models: AvailableModel[]): string {
  return JSON.stringify(
    {
      models: {
        mode: 'merge',
        providers: {
          'amp-manager': {
            baseUrl: apiBaseUrl,
            apiKey,
            api: 'openai-responses',
            models: models.map((modelItem) => ({
              id: modelItem.modelId,
              name: modelItem.modelId,
            })),
          },
        },
      },
      agents: {
        defaults: {
          model: {
            primary: `amp-manager/${models[0].modelId}`,
          },
        },
      },
    },
    null,
    2,
  )
}
