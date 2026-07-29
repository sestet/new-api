import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { fetchTokenKey, getApiKeys } from '@/features/keys/api'
import type { ApiKey } from '@/features/keys/types'
import { useAuthStore } from '@/stores/auth-store'

import { createDefaultOpenAIProfile } from './external/lib/apiProfiles'
import { useStore } from './external/store'

const DEFAULT_IMAGE_MODEL = 'gpt-image-2'

type PlaygroundModel = {
  id: string
}

const runtimeTokenKeys = new Map<number, string>()
const runtimeTokenKeyRequests = new Map<
  number,
  ReturnType<typeof fetchTokenKey>
>()

function isUsableToken(token: ApiKey): boolean {
  if (token.status !== 1) return false
  if (
    token.expired_time > 0 &&
    token.expired_time <= Math.floor(Date.now() / 1000)
  ) {
    return false
  }
  return token.unlimited_quota || token.remain_quota > 0
}

function getTokenStorageKey(userId: number | undefined): string {
  return `tlabcode-image-playground:token:${userId ?? 'anonymous'}`
}

async function fetchModels(apiKey: string): Promise<PlaygroundModel[]> {
  const response = await fetch('/v1/models', {
    headers: { Authorization: `Bearer ${apiKey}` },
    cache: 'no-store',
  })
  const payload = (await response.json().catch(() => null)) as {
    data?: unknown
    error?: { message?: string }
  } | null
  if (!response.ok) {
    throw new Error(payload?.error?.message || `HTTP ${response.status}`)
  }

  const models = Array.isArray(payload?.data) ? payload.data : []
  return models
    .map((model) => {
      if (typeof model === 'string') return { id: model }
      if (
        model &&
        typeof model === 'object' &&
        typeof (model as { id?: unknown }).id === 'string'
      ) {
        return { id: (model as { id: string }).id }
      }
      return null
    })
    .filter((model): model is PlaygroundModel => Boolean(model?.id))
}

export function PlaygroundTokenSelector() {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const setSettings = useStore((state) => state.setSettings)
  const currentModel = useStore((state) => state.settings.model)
  const initialModelRef = useRef(currentModel)
  const [tokens, setTokens] = useState<ApiKey[]>([])
  const tokensRef = useRef<ApiKey[]>([])
  const [models, setModels] = useState<PlaygroundModel[]>([])
  const [tokenId, setTokenId] = useState('')
  const [loadingTokens, setLoadingTokens] = useState(true)
  const [loadingModels, setLoadingModels] = useState(false)
  const [error, setError] = useState('')

  const applyProfile = useCallback(
    (token: ApiKey, apiKey: string, model: string) => {
      // The integrated playground uses the host application's same-origin relay.
      // Keep this explicit so the external profile validator does not treat the
      // built-in relative `/v1` route as an incomplete user configuration.
      const relayBaseUrl = `${window.location.origin}/v1`
      const profile = createDefaultOpenAIProfile({
        id: `new-api-token-${token.id}`,
        name: token.name,
        baseUrl: relayBaseUrl,
        apiKey,
        model,
        apiMode: 'images',
        apiProxy: false,
        streamImages: false,
        responseFormatB64Json: true,
      })
      setSettings({
        profiles: [profile],
        activeProfileId: profile.id,
        baseUrl: relayBaseUrl,
        apiKey,
        model,
        apiMode: 'images',
        apiProxy: false,
      })
    },
    [setSettings]
  )

  const activateToken = useCallback(
    async (
      nextTokenId: string,
      preferredModel?: string,
      tokenList?: ApiKey[]
    ) => {
      const token = (tokenList ?? tokensRef.current).find(
        (item) => String(item.id) === nextTokenId
      )
      if (!token) return

      setTokenId(nextTokenId)
      setLoadingModels(true)
      setError('')
      try {
        let apiKey = runtimeTokenKeys.get(token.id)
        if (!apiKey) {
          let keyRequest = runtimeTokenKeyRequests.get(token.id)
          if (!keyRequest) {
            keyRequest = fetchTokenKey(token.id)
            runtimeTokenKeyRequests.set(token.id, keyRequest)
          }

          try {
            const keyResponse = await keyRequest
            apiKey = keyResponse.data?.key
            if (!keyResponse.success || !apiKey) {
              throw new Error(
                keyResponse.message || t('Failed to load API token')
              )
            }
            runtimeTokenKeys.set(token.id, apiKey)
          } finally {
            runtimeTokenKeyRequests.delete(token.id)
          }
        }

        const availableModels = await fetchModels(apiKey)
        setModels(availableModels)
        const modelIds = availableModels.map((model) => model.id)
        let nextModel = modelIds.find((model) => /image|dall-e/i.test(model))
        if (preferredModel && modelIds.includes(preferredModel)) {
          nextModel = preferredModel
        }
        nextModel ??= modelIds[0] ?? DEFAULT_IMAGE_MODEL
        applyProfile(token, apiKey, nextModel)
        try {
          window.localStorage.setItem(getTokenStorageKey(userId), nextTokenId)
        } catch {
          // Private browsing and disabled storage should not block the request.
        }
      } catch (activationError) {
        setModels([])
        setError(
          activationError instanceof Error
            ? activationError.message
            : t('Failed to load models')
        )
      } finally {
        setLoadingModels(false)
      }
    },
    [applyProfile, t, userId]
  )

  useEffect(() => {
    let cancelled = false
    setLoadingTokens(true)
    void getApiKeys({ p: 1, size: 100 })
      .then((response) => {
        if (cancelled) return
        const available = response.data?.items.filter(isUsableToken) ?? []
        tokensRef.current = available
        setTokens(available)
        let preferredId = ''
        try {
          preferredId =
            window.localStorage.getItem(getTokenStorageKey(userId)) ?? ''
        } catch {
          preferredId = ''
        }
        const initial =
          available.find((token) => String(token.id) === preferredId) ??
          available[0]
        if (initial) {
          void activateToken(
            String(initial.id),
            initialModelRef.current,
            available
          )
        }
      })
      .catch((loadError) => {
        if (!cancelled) {
          setError(
            loadError instanceof Error
              ? loadError.message
              : t('Failed to load API tokens')
          )
        }
      })
      .finally(() => {
        if (!cancelled) setLoadingTokens(false)
      })

    return () => {
      cancelled = true
    }
  }, [activateToken, t, userId])

  const modelOptions = useMemo(() => {
    if (models.length) return models
    return [{ id: DEFAULT_IMAGE_MODEL }]
  }, [models])
  const selectedModel = currentModel || DEFAULT_IMAGE_MODEL

  return (
    <div className='flex min-w-0 items-center gap-2'>
      <label className='sr-only' htmlFor='gpt-image-token'>
        {t('API token')}
      </label>
      <select
        id='gpt-image-token'
        value={tokenId}
        onChange={(event) => void activateToken(event.target.value)}
        disabled={loadingTokens || !tokens.length || loadingModels}
        className='border-border bg-background text-foreground focus:ring-ring h-9 max-w-[12rem] rounded-md border px-2 text-xs outline-none focus:ring-2'
      >
        <option value=''>
          {loadingTokens ? t('Loading API tokens') : t('Select an API token')}
        </option>
        {tokens.map((token) => (
          <option key={token.id} value={token.id}>
            {token.name || `#${token.id}`}
          </option>
        ))}
      </select>

      <label className='sr-only' htmlFor='gpt-image-model'>
        {t('Model')}
      </label>
      <select
        id='gpt-image-model'
        value={selectedModel}
        onChange={(event) => {
          setSettings({ model: event.target.value })
        }}
        disabled={!tokenId || loadingModels}
        className='border-border bg-background text-foreground focus:ring-ring h-9 max-w-[12rem] rounded-md border px-2 text-xs outline-none focus:ring-2'
      >
        {modelOptions.map((model) => (
          <option key={model.id} value={model.id}>
            {model.id}
          </option>
        ))}
      </select>

      {error && (
        <span
          className='text-destructive max-w-[16rem] truncate text-xs'
          title={error}
        >
          {error}
        </span>
      )}
    </div>
  )
}
