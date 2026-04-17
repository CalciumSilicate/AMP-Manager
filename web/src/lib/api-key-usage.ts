export type CCSwitchApp = 'codex' | 'opencode' | 'openclaw'

const CODEX_PROVIDER_NAME = 'OpenAI'

interface ModelLimit {
  context: number
  output: number
}

export interface APIKeyUsageContent {
  defaultModel: string
  models: string[]
  ccSwitchLinks: Record<CCSwitchApp, string>
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
  origin,
  apiBaseUrl,
  apiKey,
  signal,
}: {
  origin: string
  apiBaseUrl: string
  apiKey: string
  signal?: AbortSignal
}): Promise<string[]> {
  const candidates = [
    `${origin}/api/provider/openai/v1/models`,
    `${apiBaseUrl}/models`,
  ]

  for (const url of candidates) {
    try {
      const response = await fetch(url, {
        method: 'GET',
        signal,
        headers: {
          Authorization: `Bearer ${apiKey}`,
          'X-Api-Key': apiKey,
        },
      })

      if (!response.ok) continue

      const payload = await response.json() as { data?: Array<{ id?: string }> }
      const models = Array.from(
        new Set(
          (payload.data || [])
            .map((item) => item.id?.trim())
            .filter((value): value is string => Boolean(value)),
        ),
      )

      if (models.length > 0) {
        return models
      }
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') {
        throw error
      }
    }
  }

  throw new Error('获取模型列表失败')
}

export function buildAPIKeyUsageContent({
  origin,
  apiBaseUrl,
  apiKey,
  keyName,
  models,
}: {
  origin: string
  apiBaseUrl: string
  apiKey: string
  keyName: string
  models: string[]
}): APIKeyUsageContent {
  const defaultModel = models[0]

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
    codex: {
      configToml: buildCodexConfigToml(apiBaseUrl, defaultModel, false),
      authJson: buildCodexAuthJson(apiKey),
      command: 'codex',
    },
    codexWebsocket: {
      configToml: buildCodexConfigToml(apiBaseUrl, defaultModel, true),
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
  const inlineConfig = buildCCSwitchInlineConfig(app, apiBaseUrl, apiKey, defaultModel)

  const params = new URLSearchParams({
    resource: 'provider',
    app,
    name: `AMP Manager (${keyName})`,
    homepage: origin,
    endpoint: apiBaseUrl,
    apiKey,
    model: defaultModel,
    enabled: 'true',
    configFormat: 'json',
    config: encodeBase64Utf8(JSON.stringify(inlineConfig)),
  })

  return `ccswitch://v1/import?${params.toString()}`
}

function buildCCSwitchInlineConfig(app: CCSwitchApp, apiBaseUrl: string, apiKey: string, defaultModel: string): Record<string, unknown> {
  switch (app) {
    case 'codex':
      return {
        auth: {
          OPENAI_API_KEY: apiKey,
        },
        config: buildCodexConfigToml(apiBaseUrl, defaultModel, true),
        meta: {
          testConfig: {
            enabled: true,
            testModel: defaultModel,
          },
        },
      }
    case 'opencode':
      return {
        options: {
          baseURL: apiBaseUrl,
          apiKey,
        },
        models: {
          [defaultModel]: {
            name: defaultModel,
          },
        },
        meta: {
          testConfig: {
            enabled: true,
            testModel: defaultModel,
          },
        },
      }
    case 'openclaw':
      return {
        baseUrl: apiBaseUrl,
        apiKey,
        api: 'openai-responses',
        models: [
          {
            id: defaultModel,
            name: defaultModel,
          },
        ],
        meta: {
          testConfig: {
            enabled: true,
            testModel: defaultModel,
          },
        },
      }
  }
}

function buildCodexConfigToml(apiBaseUrl: string, defaultModel: string, websocket: boolean): string {
  const featureSection = websocket
    ? `

[features]
responses_websockets_v2 = true`
    : ''
  const websocketField = websocket ? '\nsupports_websockets = true' : ''

  return `model_provider = "${CODEX_PROVIDER_NAME}"
model = "${defaultModel}"
review_model = "${defaultModel}"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 1000000
model_auto_compact_token_limit = 900000

[model_providers.${CODEX_PROVIDER_NAME}]
name = "${CODEX_PROVIDER_NAME}"
base_url = "${apiBaseUrl}"
wire_api = "responses"
requires_openai_auth = true${websocketField}${featureSection}
`
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

function buildOpencodeConfig(apiBaseUrl: string, apiKey: string, models: string[]): string {
  const modelEntries = Object.fromEntries(
    models.map((modelId) => [
      modelId,
      {
        name: modelId,
        limit: {
          context: getModelLimits(modelId).context,
          output: getModelLimits(modelId).output,
        },
        options: {
          store: false,
        },
        variants: {
          low: {},
          medium: {},
          high: {},
          xhigh: {},
        },
      },
    ]),
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

function buildOpenclawConfig(apiBaseUrl: string, apiKey: string, models: string[]): string {
  return JSON.stringify(
    {
      models: {
        mode: 'merge',
        providers: {
          'amp-manager': {
            baseUrl: apiBaseUrl,
            apiKey,
            api: 'openai-responses',
            models: models.map((modelId) => ({
              id: modelId,
              name: modelId,
            })),
          },
        },
      },
      agents: {
        defaults: {
          model: {
            primary: `amp-manager/${models[0]}`,
          },
        },
      },
    },
    null,
    2,
  )
}

function getModelLimits(modelId: string): ModelLimit {
  const normalized = modelId.toLowerCase()

  if (normalized.startsWith('gpt-4.1')) {
    return { context: 1_047_576, output: 32_768 }
  }
  if (normalized.startsWith('gpt-5')) {
    return { context: 400_000, output: 128_000 }
  }
  if (normalized.includes('gpt-5-codex') || normalized.includes('codex')) {
    return { context: 400_000, output: 128_000 }
  }
  if (normalized.startsWith('gpt-4') || normalized.startsWith('gpt-4o')) {
    return { context: 128_000, output: 16_384 }
  }
  if (normalized.startsWith('claude-4') || normalized.includes('claude-sonnet') || normalized.includes('claude-opus') || normalized.includes('claude-haiku')) {
    return { context: 200_000, output: 64_000 }
  }
  if (normalized.startsWith('claude-3') || normalized.startsWith('claude')) {
    return { context: 200_000, output: 8_192 }
  }
  if (normalized.startsWith('gemini-2.5') || normalized.startsWith('gemini-3') || normalized.startsWith('gemini')) {
    return { context: 1_048_576, output: 65_536 }
  }
  if (normalized.startsWith('deepseek')) {
    return { context: 128_000, output: 8_192 }
  }
  if (normalized.startsWith('qwen3')) {
    return { context: 32_768, output: 8_192 }
  }

  return { context: 128_000, output: 32_768 }
}

function encodeBase64Utf8(value: string): string {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }
  return btoa(binary)
}
