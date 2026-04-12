<template>
  <div class="space-y-6">
    <div class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-gray-800 dark:bg-gray-900">
      <div class="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-gray-100">Gateway Audit</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            Structured gateway audit events for tool-call responses, risk flags, and canary injection.
          </p>
        </div>
        <button
          class="inline-flex items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-gray-700 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-300"
          :disabled="loading"
          @click="loadEvents"
        >
          {{ loading ? 'Loading...' : 'Refresh' }}
        </button>
      </div>

      <div class="mt-5 grid gap-3 md:grid-cols-6">
        <input v-model="filters.q" class="rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-gray-700 dark:bg-gray-950" placeholder="Search request/model/path" />
        <input v-model="filters.request_id" class="rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-gray-700 dark:bg-gray-950" placeholder="Request ID" />
        <input v-model="filters.path" class="rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-gray-700 dark:bg-gray-950" placeholder="Path" />
        <select v-model="filters.platform" class="rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-gray-700 dark:bg-gray-950">
          <option value="">All platforms</option>
          <option value="openai">openai</option>
          <option value="anthropic">anthropic</option>
          <option value="gemini">gemini</option>
          <option value="antigravity">antigravity</option>
          <option value="sora">sora</option>
        </select>
        <select v-model="filters.risk_level" class="rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-gray-700 dark:bg-gray-950">
          <option value="">All risk levels</option>
          <option value="low">low</option>
          <option value="medium">medium</option>
          <option value="high">high</option>
        </select>
        <select v-model="toolCallsFilter" class="rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-gray-700 dark:bg-gray-950">
          <option value="">Any tool call state</option>
          <option value="true">Has tool calls</option>
          <option value="false">No tool calls</option>
        </select>
        <select v-model="canaryFilter" class="rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-gray-700 dark:bg-gray-950">
          <option value="">Any canary state</option>
          <option value="true">Canary injected</option>
          <option value="false">No canary</option>
        </select>
      </div>
    </div>

    <div class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-gray-800 dark:bg-gray-900">
      <div v-if="error" class="border-b border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/40 dark:text-red-300">
        {{ error }}
      </div>
      <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-gray-800">
          <thead class="bg-gray-50 dark:bg-gray-950">
            <tr class="text-left text-xs uppercase tracking-wide text-gray-500 dark:text-gray-400">
              <th class="px-4 py-3">Time</th>
              <th class="px-4 py-3">Platform</th>
              <th class="px-4 py-3">Path</th>
              <th class="px-4 py-3">Model</th>
              <th class="px-4 py-3">Status</th>
              <th class="px-4 py-3">Risk</th>
              <th class="px-4 py-3">Tools</th>
              <th class="px-4 py-3">Canary</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 text-sm dark:divide-gray-800">
            <tr
              v-for="event in events"
              :key="event.id"
              class="cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-950/70"
              @click="openEvent(event.id)"
            >
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ formatDate(event.created_at) }}</td>
              <td class="px-4 py-3">
                <span class="rounded-full bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-gray-800 dark:text-gray-200">{{ event.platform || 'unknown' }}</span>
              </td>
              <td class="px-4 py-3 font-mono text-xs text-gray-700 dark:text-gray-200">{{ event.path }}</td>
              <td class="px-4 py-3 text-gray-700 dark:text-gray-200">{{ event.effective_model || event.requested_model || '—' }}</td>
              <td class="px-4 py-3 text-gray-700 dark:text-gray-200">{{ event.status_code }}</td>
              <td class="px-4 py-3">
                <span :class="riskClass(event.risk_level)" class="rounded-full px-2 py-1 text-xs font-medium">{{ event.risk_level }}</span>
              </td>
              <td class="px-4 py-3 text-gray-700 dark:text-gray-200">{{ event.tool_count }}</td>
              <td class="px-4 py-3 text-gray-700 dark:text-gray-200">{{ event.canary_injected ? 'yes' : 'no' }}</td>
            </tr>
            <tr v-if="!loading && events.length === 0">
              <td colspan="8" class="px-4 py-8 text-center text-sm text-gray-500 dark:text-gray-400">
                No audit events found.
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div
      v-if="selected"
      class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-gray-800 dark:bg-gray-900"
    >
      <div class="flex items-start justify-between gap-4">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">Event #{{ selected.id }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ selected.request_id || 'No request id' }}</p>
        </div>
        <button class="text-sm text-gray-500 hover:text-gray-900 dark:hover:text-gray-100" @click="selected = null">Close</button>
      </div>

      <div class="mt-5 grid gap-4 md:grid-cols-2">
        <div class="rounded-xl border border-gray-200 p-4 dark:border-gray-800">
          <div class="space-y-2 text-sm text-gray-700 dark:text-gray-200">
            <div><strong>Inbound:</strong> {{ selected.inbound_endpoint || '—' }}</div>
            <div><strong>Upstream:</strong> {{ selected.upstream_endpoint || '—' }}</div>
            <div><strong>Target:</strong> {{ selected.upstream_target || '—' }}</div>
            <div><strong>Request hash:</strong> <span class="font-mono text-xs">{{ selected.request_hash || '—' }}</span></div>
            <div><strong>Response hash:</strong> <span class="font-mono text-xs">{{ selected.response_hash || '—' }}</span></div>
          </div>
        </div>
        <div class="rounded-xl border border-gray-200 p-4 dark:border-gray-800">
          <div class="space-y-2 text-sm text-gray-700 dark:text-gray-200">
            <div><strong>Risk flags:</strong> {{ (selected.risk_flags || []).join(', ') || '—' }}</div>
            <div><strong>Canary labels:</strong> {{ (selected.canary_labels || []).join(', ') || '—' }}</div>
            <div><strong>User agent:</strong> {{ selected.user_agent || '—' }}</div>
            <div><strong>Tool count:</strong> {{ selected.tool_count }}</div>
          </div>
        </div>
      </div>

      <div class="mt-5 rounded-xl border border-gray-200 p-4 dark:border-gray-800">
        <h3 class="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">Tool Calls</h3>
        <div v-if="!(selected.tool_calls || []).length" class="text-sm text-gray-500 dark:text-gray-400">No tool calls captured.</div>
        <div v-for="(tool, index) in selected.tool_calls || []" :key="index" class="mb-4 rounded-lg bg-gray-50 p-3 dark:bg-gray-950">
          <div class="font-medium text-gray-900 dark:text-gray-100">{{ tool.name }}</div>
          <pre class="mt-2 overflow-x-auto whitespace-pre-wrap break-all text-xs text-gray-700 dark:text-gray-300">{{ JSON.stringify(tool.arguments ?? {}, null, 2) }}</pre>
        </div>
        <div v-if="(selected.tool_hashes || []).length" class="mt-4 border-t border-gray-200 pt-4 dark:border-gray-800">
          <h4 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">Tool Hashes</h4>
          <div v-for="(toolHash, index) in selected.tool_hashes || []" :key="toolHash + index" class="font-mono text-xs text-gray-600 dark:text-gray-300">
            {{ toolHash }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { adminAPI } from '@/api/admin'
import type { AuditEvent, AuditEventQuery } from '@/api/admin/ops'

const events = ref<AuditEvent[]>([])
const selected = ref<AuditEvent | null>(null)
const loading = ref(false)
const error = ref('')

const filters = ref<AuditEventQuery>({
  page: 1,
  page_size: 50,
  q: '',
  platform: '',
  risk_level: ''
})

const toolCallsFilter = ref('')
const canaryFilter = ref('')

const query = computed<AuditEventQuery>(() => ({
  ...filters.value,
  has_tool_calls: toolCallsFilter.value === '' ? undefined : toolCallsFilter.value === 'true',
  canary_injected: canaryFilter.value === '' ? undefined : canaryFilter.value === 'true'
}))

async function loadEvents() {
  loading.value = true
  error.value = ''
  try {
    const response = await adminAPI.ops.listAuditEvents(query.value)
    events.value = response.items
  } catch (err: any) {
    error.value = err?.message || 'Failed to load audit events'
  } finally {
    loading.value = false
  }
}

async function openEvent(id: number) {
  try {
    selected.value = await adminAPI.ops.getAuditEvent(id)
  } catch (err: any) {
    error.value = err?.message || 'Failed to load audit event'
  }
}

function formatDate(value: string) {
  if (!value) return '—'
  return new Date(value).toLocaleString()
}

function riskClass(level: string) {
  switch (level) {
    case 'high':
      return 'bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-300'
    case 'medium':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300'
    default:
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
  }
}

watch(query, () => {
  loadEvents()
}, { deep: true })

onMounted(() => {
  loadEvents()
})
</script>
