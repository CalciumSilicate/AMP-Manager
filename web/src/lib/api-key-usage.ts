export type CCSwitchApp = 'codex' | 'opencode' | 'openclaw'

const CODEX_PROVIDER_NAME = 'OpenAI'
const GENERIC_CONTEXT_WINDOW = 1_000_000
const GENERIC_OUTPUT_LIMIT = 128_000

export interface APIKeyUsageContent {
  defaultModel: string
  models: string[]
  ccSwitchUsageScript: string
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
  const ccSwitchUsageScript = buildCCSwitchUsageScript()

  return {
    defaultModel,
    models,
    ccSwitchUsageScript,
    ccSwitchLinks: {
      codex: buildCCSwitchDeepLink({
        app: 'codex',
        origin,
        apiBaseUrl,
        apiKey,
        keyName,
        defaultModel,
        usageScript: ccSwitchUsageScript,
      }),
      opencode: buildCCSwitchDeepLink({
        app: 'opencode',
        origin,
        apiBaseUrl,
        apiKey,
        keyName,
        defaultModel,
        usageScript: ccSwitchUsageScript,
      }),
      openclaw: buildCCSwitchDeepLink({
        app: 'openclaw',
        origin,
        apiBaseUrl,
        apiKey,
        keyName,
        defaultModel,
        usageScript: ccSwitchUsageScript,
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

function buildCCSwitchUsageScript(): string {
  return `({
  request: {
    url: "{{baseUrl}}/api/usage",
    method: "GET",
    headers: {
      "Authorization": "Bearer {{apiKey}}",
      "User-Agent": "cc-switch/1.0"
    }
  },
  extractor: function(response) {
    if (Array.isArray(response)) {
      return response;
    }

    return {
      isValid: false,
      invalidMessage: response && response.error ? response.error : "查询失败"
    };
  }
})`
}

function buildCCSwitchDeepLink({
  app,
  origin,
  apiBaseUrl,
  apiKey,
  keyName,
  defaultModel,
  usageScript,
}: {
  app: CCSwitchApp
  origin: string
  apiBaseUrl: string
  apiKey: string
  keyName: string
  defaultModel: string
  usageScript: string
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
    usageEnabled: 'true',
    usageBaseUrl: origin,
    usageApiKey: apiKey,
    usageAutoInterval: '60',
    usageScript: encodeBase64Utf8(usageScript),
  })

  return `ccswitch://v1/import?${params.toString()}`
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
          context: GENERIC_CONTEXT_WINDOW,
          output: GENERIC_OUTPUT_LIMIT,
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

function encodeBase64Utf8(value: string): string {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }
  return btoa(binary)
}
