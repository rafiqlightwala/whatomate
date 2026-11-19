<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '@/services/api'
import { toast } from '@/lib/toast'
import { getErrorMessage } from '@/lib/api-utils'

interface Job {
 id: string; contact_id: string; account: string; state: string; due_at: string;
 answer: string; reason: string; decision: string; version: string; attempts: number; send_attempts: number;
 contact?: { profile_name: string; phone_number: string }
}
interface QueueData {
 policy: { enabled: boolean; paused: boolean; first_run_at: string; billing_timezone: string };
 jobs: Job[]; counts: { state: string; count: number }[];
 budgets: { account: string; used: number; limit: number; resets_at: string }[];
 monthly: { month: string; account: string; incoming_customers: number; replied_customers: number; sends: number }[];
 version: string
}
const data = ref<QueueData>()
const error = ref('')
const busy = ref(false)
const checking = ref(false)
const checks = ref<{name: string; passed: boolean; body: string}[]>([])
async function checkPrompts() {
 checking.value = true; checks.value = []
 try {
 for (const name of ['greeting','boilerplate','login','combined','resolved','roman_urdu','vague_issue','feature_request']) {
 const response = await api.post('/chatbot/support/preview', { case: name }, { timeout: 75000 })
 const result = response.data.data
 checks.value.push({name, passed: result.passed, body: `${result.prepared.decision} · ${result.prepared.language} · ${result.prepared.questions?.length || 0} questions\n${result.prepared.reason}\n${(result.prepared.questions || []).map((q: {category: string; question: string}) => `${q.category}: ${q.question}`).join('\n')}\n\n${result.prepared.body || ''}`})
 }
 } catch (err) { const message = getErrorMessage(err, 'Prompt check failed'); checks.value.push({name: 'Model error', passed: false, body: message}); toast.error(message) }
 finally { checking.value = false }
}
const selected = ref('')
const zone = ref('Asia/Karachi')
const state = ref('')
const page = ref(0)
let timer: ReturnType<typeof setInterval>
function date(value: string) { return new Date(value).toLocaleString('en-GB', { timeZone: 'Asia/Karachi', dateStyle: 'medium', timeStyle: 'short' }) + ' PKT' }
function details(job: Job) {
 try { return JSON.parse(job.decision) as { summary?: string; questions?: { question: string; attempted_steps: string[] }[] } } catch { return {} }
}
async function load() {
 try {
  const response = await api.get('/chatbot/support', { params: { state: state.value, page: page.value } })
  data.value = response.data.data
  if (data.value?.policy.billing_timezone) zone.value = data.value.policy.billing_timezone
  error.value = ''
 } catch (err) { error.value = getErrorMessage(err, 'Could not load support queue') }
}
async function toggle() {
 busy.value = true
 try {
  await api.put('/chatbot/support', { enabled: !(data.value?.policy.enabled && !data.value?.policy.paused), billing_timezone: zone.value })
  await load()
  toast.success(data.value?.policy.enabled && !data.value?.policy.paused ? 'Consolidated support enabled' : 'Consolidated support paused')
 } catch (err) { toast.error(getErrorMessage(err, 'Could not update support settings')) }
 finally { busy.value = false }
}
async function filter() { page.value = 0; await load() }
onMounted(() => { load(); timer = setInterval(load, 30000) })
onUnmounted(() => clearInterval(timer))
</script>

<template>
 <article class="card vstack gap-4" aria-labelledby="support-queue-title">
  <header class="hstack justify-between flex-wrap gap-2">
   <div>
    <h2 id="support-queue-title">Consolidated support queue</h2>
    <small class="text-light">One automated reply per customer each month · 9 AM PKT · expiry protection</small>
   </div>
   <div class="hstack gap-2">
    <button type="button" class="outline small" @click="load">Refresh queue</button>
    <button type="button" class="small" :disabled="busy || !data" @click="toggle">{{ data?.policy.enabled && !data?.policy.paused ? 'Pause consolidated support' : 'Enable consolidated support' }}</button>
   </div>
  </header>
  <div v-if="error" role="alert" data-variant="danger">{{ error }}</div>
  <template v-if="data">
   <div class="hstack flex-wrap gap-4">
    <span class="badge" :data-variant="data.policy.enabled && !data.policy.paused ? 'success' : 'warning'">{{ data.policy.enabled && !data.policy.paused ? 'Active' : 'Paused' }}</span>
    <small v-if="data.policy.enabled">First batch: {{ date(data.policy.first_run_at) }}</small>
    <small>Knowledge: {{ data.version }}</small>
    <label class="hstack gap-2"><small>Billing timezone</small><input v-model="zone" class="w-auto" :disabled="data.policy.enabled" aria-label="Billing timezone"></label>
   </div>
   <small class="text-light">Pending work survives restarts in the existing database. Expiring windows may send eight minutes early. Manual replies remove the customer from automation for the month. Pausing stops automated sends while continuing to collect questions. Legacy keyword replies stay disabled.</small>
   <div class="row">
    <div v-for="budget in data.budgets" :key="budget.account" class="col-6">
     <div class="card vstack gap-1">
      <strong>{{ budget.account }}: {{ budget.used }} / {{ budget.limit }}</strong>
      <div v-if="budget.used >= budget.limit" role="alert" data-variant="danger">Monthly cap reached. Sending is paused.</div>
      <div v-else-if="budget.used >= 950" role="alert" data-variant="warning">Fewer than 50 sends remain.</div>
      <div v-else-if="budget.used >= 800" role="alert" data-variant="warning">Monthly allowance is running low.</div>
      <small class="text-light">Includes manual sends and uncertain outcomes. Resets {{ date(budget.resets_at) }}.</small>
     </div>
    </div>
   </div>
   <div class="hstack flex-wrap gap-2"><span v-for="count in data.counts" :key="count.state" class="badge">{{ count.state }}: {{ count.count }}</span></div>
   <div class="hstack flex-wrap gap-2">
    <label class="hstack gap-2"><small>Show</small><select v-model="state" class="w-auto" @change="filter"><option value="">All states</option><option v-for="s in ['pending','ready','skipped','suppressed','sent','expired','error','uncertain','send_failed','budget_paused']" :key="s" :value="s">{{ s }}</option></select></label>
    <small class="text-light">Updates every 30 seconds. Select a row to inspect the answer.</small>
   </div>
   <p v-if="!data.jobs?.length" class="text-light">No conversations in this view yet. New questions will appear here after consolidated support is enabled.</p>
   <div v-for="job in data.jobs" :key="job.id" class="card">
    <button type="button" class="ghost w-100 hstack justify-between flex-wrap gap-2 align-left" :aria-expanded="selected === job.id" @click="selected = selected === job.id ? '' : job.id">
     <span class="vstack gap-1 align-left"><strong>{{ job.contact?.profile_name || job.contact?.phone_number || 'Customer' }}</strong><small class="text-light">{{ job.reason || 'Waiting for classification' }}</small></span>
     <span class="vstack gap-1 align-right"><small>{{ job.state }}</small><small class="text-light">{{ date(job.due_at) }}</small></span>
    </button>
    <div v-if="selected === job.id" class="vstack gap-2 mt-2">
     <RouterLink :to="`/chat/${job.contact_id}`">Open conversation</RouterLink>
     <p>{{ details(job).summary }}</p>
     <ol><li v-for="(q, index) in details(job).questions" :key="index">{{ q.question }}<small v-if="q.attempted_steps?.length" class="text-light"> Already tried: {{ q.attempted_steps.join('; ') }}</small></li></ol>
     <h3>Prepared reply</h3><p class="pre-wrap">{{ job.answer || 'No reply prepared for this state.' }}</p>
     <small class="text-light">{{ job.account }} · {{ job.version }} · AI retries: {{ job.attempts }} · rejected sends: {{ job.send_attempts }}</small>
    </div>
   </div>
   <div class="hstack gap-2"><button type="button" class="outline small" :disabled="page === 0" @click="page--; load()">Previous</button><button type="button" class="outline small" :disabled="(data.jobs?.length || 0) < 50" @click="page++; load()">Next</button></div>
   <details><summary>Prompt checks</summary>
    <div class="vstack gap-2">
     <small class="text-light">Runs eight synthetic cases using the configured model. No WhatsApp messages are sent.</small>
     <div><button type="button" class="outline small" :disabled="checking" :aria-busy="checking ? 'true' : undefined" @click="checkPrompts">{{ checking ? 'Checking prompts…' : 'Run prompt checks' }}</button></div>
     <details v-for="check in checks" :key="check.name"><summary>{{ check.passed ? 'Passed' : 'Needs attention' }}: {{ check.name }}</summary><p class="pre-wrap">{{ check.body }}</p></details>
    </div>
   </details>
   <details><summary>Monthly customer counts</summary>
    <div class="scroll-x"><table><thead><tr><th>Month (PKT)</th><th>Number</th><th>Customers messaging</th><th>Customers replied to</th><th>Successful sends</th></tr></thead><tbody><tr v-for="row in data.monthly" :key="row.month + row.account"><td>{{ row.month }}</td><td>{{ row.account }}</td><td>{{ row.incoming_customers }}</td><td>{{ row.replied_customers }}</td><td>{{ row.sends }}</td></tr></tbody></table></div>
   </details>
  </template>
 </article>
</template>
