<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '@/services/api'
import { Button } from '@/components/ui/button'
import { toast } from 'vue-sonner'
import { getErrorMessage } from '@/lib/api-utils'

interface Job {
 id: string; contact_id: string; account: string; state: string; due_at: string;
 answer: string; reason: string; decision: string; version: string;
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
 for (const name of ['greeting','boilerplate','login','combined','resolved','roman_urdu']) {
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
 <section class="rounded-xl border border-white/10 bg-white/[0.02] p-5 space-y-5 light:bg-white light:border-gray-200">
  <div class="flex flex-wrap justify-between gap-3">
   <div><h2 class="text-xl font-semibold">Consolidated support queue</h2><p class="text-sm opacity-70">One automated reply per customer each month · 9 AM PKT · expiry protection</p></div>
   <div class="flex gap-2"><Button variant="outline" @click="load">Refresh queue</Button><Button :disabled="busy || !data" @click="toggle">{{ data?.policy.enabled && !data?.policy.paused ? 'Pause consolidated support' : 'Enable consolidated support' }}</Button></div>
  </div>
  <p v-if="error" role="alert" class="text-red-400">{{ error }}</p>
  <template v-if="data">
   <div class="flex flex-wrap gap-4 text-sm">
    <strong>{{ data.policy.enabled && !data.policy.paused ? 'Active' : 'Paused' }}</strong>
    <span v-if="data.policy.enabled">First batch: {{ date(data.policy.first_run_at) }}</span>
    <span>Knowledge: {{ data.version }}</span>
    <label>Billing timezone <input v-model="zone" :disabled="data.policy.enabled" class="rounded border bg-transparent p-1" aria-label="Billing timezone"></label>
   </div>
   <p class="text-sm opacity-70">Pending work survives restarts in the existing database. Expiring windows may send eight minutes early. Manual replies remove the customer from automation for the month. Pausing stops automated sends while continuing to collect questions. Legacy keyword replies stay disabled.</p>
   <div class="grid gap-3 md:grid-cols-2">
    <div v-for="budget in data.budgets" :key="budget.account" class="rounded border border-white/10 light:border-gray-200 p-3">
     <strong>{{ budget.account }}: {{ budget.used }} / {{ budget.limit }}</strong>
     <p v-if="budget.used >= budget.limit" class="text-red-400">Monthly cap reached. Sending is paused.</p>
     <p v-else-if="budget.used >= 950" class="text-amber-400">Fewer than 50 sends remain.</p>
     <p v-else-if="budget.used >= 800" class="text-amber-400">Monthly allowance is running low.</p>
     <p class="text-xs opacity-70">Includes manual sends and uncertain outcomes. Resets {{ date(budget.resets_at) }}.</p>
    </div>
   </div>
   <div class="flex flex-wrap gap-3 text-sm"><span v-for="count in data.counts" :key="count.state">{{ count.state }}: <strong>{{ count.count }}</strong></span></div>
   <div class="flex items-center gap-3"><label>Show <select v-model="state" class="bg-transparent border rounded p-2" @change="filter"><option value="">All states</option><option v-for="s in ['pending','ready','skipped','suppressed','sent','expired','error','uncertain','budget_paused']" :key="s" :value="s">{{ s }}</option></select></label><span class="text-xs opacity-60">Updates every 30 seconds. Select a row to inspect the answer.</span></div>
   <p v-if="!data.jobs?.length" class="py-4 opacity-70">No conversations in this view yet. New questions will appear here after consolidated support is enabled.</p>
   <div v-for="job in data.jobs" :key="job.id" class="border border-white/10 light:border-gray-200 rounded-lg">
    <button class="w-full p-3 text-left flex flex-wrap justify-between gap-2 hover:bg-white/5" :aria-expanded="selected === job.id" @click="selected = selected === job.id ? '' : job.id">
     <span><strong>{{ job.contact?.profile_name || job.contact?.phone_number || 'Customer' }}</strong><span class="block text-sm opacity-70">{{ job.reason || 'Waiting for classification' }}</span></span>
     <span class="text-sm">{{ job.state }}<span class="block opacity-60">{{ date(job.due_at) }}</span></span>
    </button>
    <div v-if="selected === job.id" class="p-4 space-y-3 border-t border-white/10 light:border-gray-200">
     <RouterLink :to="`/chat/${job.contact_id}`" class="text-blue-400 underline">Open conversation</RouterLink>
     <p>{{ details(job).summary }}</p>
     <ol class="list-decimal pl-5"><li v-for="(q, index) in details(job).questions" :key="index">{{ q.question }}<p v-if="q.attempted_steps?.length" class="text-sm opacity-60">Already tried: {{ q.attempted_steps.join('; ') }}</p></li></ol>
     <h3 class="font-semibold">Prepared reply</h3><p class="whitespace-pre-wrap text-sm">{{ job.answer || 'No reply prepared for this state.' }}</p>
     <p class="text-xs opacity-60">{{ job.account }} · {{ job.version }}</p>
    </div>
   </div>
   <div class="flex gap-2"><Button variant="outline" :disabled="page === 0" @click="page--; load()">Previous</Button><Button variant="outline" :disabled="(data.jobs?.length || 0) < 50" @click="page++; load()">Next</Button></div>
   <details><summary class="cursor-pointer font-semibold">Prompt checks</summary>
    <p class="text-sm opacity-70 my-2">Runs six synthetic cases using the configured model. No WhatsApp messages are sent.</p>
    <Button variant="outline" :disabled="checking" @click="checkPrompts">{{ checking ? 'Checking prompts…' : 'Run prompt checks' }}</Button>
    <details v-for="check in checks" :key="check.name" class="mt-2"><summary>{{ check.passed ? 'Passed' : 'Needs attention' }}: {{ check.name }}</summary><p class="whitespace-pre-wrap text-sm p-2">{{ check.body }}</p></details>
   </details>
   <details><summary class="cursor-pointer font-semibold">Monthly customer counts</summary>
    <div class="overflow-x-auto"><table class="w-full text-sm mt-3 text-left"><thead><tr><th>Month (PKT)</th><th>Number</th><th>Customers messaging</th><th>Customers replied to</th><th>Successful sends</th></tr></thead><tbody><tr v-for="row in data.monthly" :key="row.month + row.account"><td class="py-2">{{ row.month }}</td><td>{{ row.account }}</td><td>{{ row.incoming_customers }}</td><td>{{ row.replied_customers }}</td><td>{{ row.sends }}</td></tr></tbody></table></div>
   </details>
  </template>
 </section>
</template>
