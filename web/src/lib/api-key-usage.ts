const DEFAULT_MODEL = 'gpt-5.4'

export type CCSwitchApp = 'codex' | 'opencode' | 'openclaw'

export interface APIKeyUsageContent {
  defaultModel: string
  usageEndpoint: string
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

export function buildAPIKeyUsageContent({
  origin,
  apiBaseUrl,
  apiKey,
  keyName,
}: {
  origin: string
  apiBaseUrl: string
  apiKey: string
  keyName: string
}): APIKeyUsageContent {
  const ccSwitchUsageScript = buildCCSwitchUsageScript()

  return {
    defaultModel: DEFAULT_MODEL,
    usageEndpoint: `${origin}/api/usage`,
    ccSwitchUsageScript,
    ccSwitchLinks: {
      codex: buildCCSwitchDeepLink({
        app: 'codex',
        origin,
        apiBaseUrl,
        apiKey,
        keyName,
        usageScript: ccSwitchUsageScript,
      }),
      opencode: buildCCSwitchDeepLink({
        app: 'opencode',
        origin,
        apiBaseUrl,
        apiKey,
        keyName,
        usageScript: ccSwitchUsageScript,
      }),
      openclaw: buildCCSwitchDeepLink({
        app: 'openclaw',
        origin,
        apiBaseUrl,
        apiKey,
        keyName,
        usageScript: ccSwitchUsageScript,
      }),
    },
    codex: {
      configToml: buildCodexConfigToml(apiBaseUrl, false),
      authJson: buildCodexAuthJson(apiKey),
      command: 'codex',
    },
    codexWebsocket: {
      configToml: buildCodexConfigToml(apiBaseUrl, true),
      authJson: buildCodexAuthJson(apiKey),
      command: 'codex',
    },
    opencode: {
      configJson: buildOpencodeConfig(apiBaseUrl, apiKey),
      command: `opencode -m openai/${DEFAULT_MODEL}`,
    },
    openclaw: {
      configJson: buildOpenclawConfig(apiBaseUrl, apiKey),
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
  usageScript,
}: {
  app: CCSwitchApp
  origin: string
  apiBaseUrl: string
  apiKey: string
  keyName: string
  usageScript: string
}): string {
  const params = new URLSearchParams({
    resource: 'provider',
    app,
    name: `AMP Manager (${keyName})`,
    homepage: origin,
    endpoint: apiBaseUrl,
    apiKey,
    model: DEFAULT_MODEL,
    enabled: 'true',
    usageEnabled: 'true',
    usageBaseUrl: origin,
    usageApiKey: apiKey,
    usageAutoInterval: '60',
    usageScript: encodeBase64Utf8(usageScript),
  })

  return `ccswitch://v1/import?${params.toString()}`
}

function buildCodexConfigToml(apiBaseUrl: string, websocket: boolean): string {
  const featureSection = websocket
    ? `
[features]
responses_websockets_v2 = true
`
    : ''
  const websocketFields = websocket
    ? `
supports_websockets = true`
    : ''

  return `model_provider = "OpenAI"
model = "${DEFAULT_MODEL}"${featureSection}
[model_providers.OpenAI]
name = "OpenAI"
base_url = "${apiBaseUrl}"
wire_api = "responses"
requires_openai_auth = true${websocketFields}
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

function buildOpencodeConfig(apiBaseUrl: string, apiKey: string): string {
  return JSON.stringify(
    {
      provider: {
        openai: {
          options: {
            baseURL: apiBaseUrl,
            apiKey,
          },
          models: {
            [DEFAULT_MODEL]: {
              name: 'GPT-5.4',
              limit: {
                context: 1_050_000,
                output: 128_000,
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
          },
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

function buildOpenclawConfig(apiBaseUrl: string, apiKey: string): string {
  return JSON.stringify(
    {
      models: {
        mode: 'merge',
        providers: {
          'amp-manager': {
            baseUrl: apiBaseUrl,
            apiKey,
            api: 'openai-responses',
            models: [
              {
                id: DEFAULT_MODEL,
                name: DEFAULT_MODEL,
              },
            ],
          },
        },
      },
      agents: {
        defaults: {
          model: {
            primary: `amp-manager/${DEFAULT_MODEL}`,
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

