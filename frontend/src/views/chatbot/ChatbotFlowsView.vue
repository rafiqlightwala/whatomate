<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { chatbotService, flowsService } from '@/services/api'
import { toast } from 'vue-sonner'
import { Plus, Pencil, Trash2, Workflow, ArrowLeft, Play, Pause, GripVertical, ChevronDown, ChevronUp } from 'lucide-vue-next'

interface ApiConfig {
  url: string
  method: string
  headers: Record<string, string>
  body: string
  response_path: string
  fallback_message: string
}

interface ButtonConfig {
  id: string
  title: string
}

interface FlowStep {
  id?: string
  step_name: string
  step_order: number
  message: string
  message_type: string
  input_type: string
  input_config: Record<string, any>
  api_config: ApiConfig
  buttons: ButtonConfig[]
  validation_regex: string
  validation_error: string
  store_as: string
  next_step: string
  retry_on_invalid: boolean
  max_retries: number
}

interface WebhookConfig {
  url: string
  method: string
  headers: Record<string, string>
  body: string
}

interface ChatbotFlow {
  id: string
  name: string
  description: string
  trigger_keywords: string[]
  steps_count: number
  enabled: boolean
  created_at: string
  initial_message: string
  completion_message: string
  on_complete_action: string
  completion_config: WebhookConfig
  steps?: FlowStep[]
}

interface WhatsAppFlow {
  id: string
  name: string
  status: string
  meta_flow_id: string
}

const flows = ref<ChatbotFlow[]>([])
const whatsappFlows = ref<WhatsAppFlow[]>([])
const isLoading = ref(true)
const isDialogOpen = ref(false)
const isSubmitting = ref(false)
const editingFlow = ref<ChatbotFlow | null>(null)
const expandedStep = ref<number | null>(null)
const deleteDialogOpen = ref(false)
const flowToDelete = ref<ChatbotFlow | null>(null)

const defaultWebhookConfig: WebhookConfig = {
  url: '',
  method: 'POST',
  headers: {},
  body: ''
}

const formData = ref({
  name: '',
  description: '',
  trigger_keywords: '',
  initial_message: '',
  completion_message: '',
  on_complete_action: 'none',
  completion_config: { ...defaultWebhookConfig },
  enabled: true,
  steps: [] as FlowStep[]
})

const defaultApiConfig: ApiConfig = {
  url: '',
  method: 'GET',
  headers: {},
  body: '',
  response_path: '',
  fallback_message: ''
}

const defaultStep: FlowStep = {
  step_name: '',
  step_order: 0,
  message: '',
  message_type: 'text',
  input_type: 'text',
  input_config: {},
  api_config: { ...defaultApiConfig },
  buttons: [],
  validation_regex: '',
  validation_error: 'Invalid input. Please try again.',
  store_as: '',
  next_step: '',
  retry_on_invalid: true,
  max_retries: 3
}

onMounted(async () => {
  await Promise.all([fetchFlows(), fetchWhatsAppFlows()])
})

async function fetchFlows() {
  isLoading.value = true
  try {
    const response = await chatbotService.listFlows()
    const data = response.data.data || response.data
    flows.value = data.flows || []
  } catch (error) {
    console.error('Failed to load flows:', error)
    flows.value = []
  } finally {
    isLoading.value = false
  }
}

async function fetchWhatsAppFlows() {
  try {
    const response = await flowsService.list()
    const data = response.data.data || response.data
    const allFlows = data.flows || []
    // Only show published flows that have a meta_flow_id
    whatsappFlows.value = allFlows.filter(
      (f: WhatsAppFlow) => f.meta_flow_id && f.status?.toUpperCase() === 'PUBLISHED'
    )
  } catch (error) {
    console.error('Failed to load WhatsApp flows:', error)
    whatsappFlows.value = []
  }
}

function openCreateDialog() {
  editingFlow.value = null
  formData.value = {
    name: '',
    description: '',
    trigger_keywords: '',
    initial_message: 'Hi! Let me help you with that.',
    completion_message: 'Thank you! We have all the information we need.',
    on_complete_action: 'none',
    completion_config: { ...defaultWebhookConfig },
    enabled: true,
    steps: [{ ...defaultStep, step_name: 'step_1', step_order: 1, message: 'What is your name?', store_as: 'name' }]
  }
  expandedStep.value = 0
  isDialogOpen.value = true
}

async function openEditDialog(flow: ChatbotFlow) {
  try {
    const response = await chatbotService.getFlow(flow.id)
    const fullFlow = response.data.data || response.data
    editingFlow.value = fullFlow
    formData.value = {
      name: fullFlow.name || fullFlow.Name || '',
      description: fullFlow.description || fullFlow.Description || '',
      trigger_keywords: (fullFlow.trigger_keywords || fullFlow.TriggerKeywords || []).join(', '),
      initial_message: fullFlow.initial_message || fullFlow.InitialMessage || '',
      completion_message: fullFlow.completion_message || fullFlow.CompletionMessage || '',
      on_complete_action: fullFlow.on_complete_action || fullFlow.OnCompleteAction || 'none',
      completion_config: fullFlow.completion_config || fullFlow.CompletionConfig || { ...defaultWebhookConfig },
      enabled: fullFlow.is_enabled ?? fullFlow.IsEnabled ?? fullFlow.enabled ?? true,
      steps: (fullFlow.steps || fullFlow.Steps || []).map((s: any, idx: number) => ({
        id: s.id || s.ID,
        step_name: s.step_name || s.StepName || `step_${idx + 1}`,
        step_order: s.step_order ?? s.StepOrder ?? idx + 1,
        message: s.message || s.Message || '',
        message_type: s.message_type || s.MessageType || 'text',
        input_type: s.input_type || s.InputType || 'text',
        input_config: s.input_config || s.InputConfig || {},
        api_config: s.api_config || s.ApiConfig || { ...defaultApiConfig },
        buttons: s.buttons || s.Buttons || [],
        validation_regex: s.validation_regex || s.ValidationRegex || '',
        validation_error: s.validation_error || s.ValidationError || 'Invalid input. Please try again.',
        store_as: s.store_as || s.StoreAs || '',
        next_step: s.next_step || s.NextStep || '',
        retry_on_invalid: s.retry_on_invalid ?? s.RetryOnInvalid ?? true,
        max_retries: s.max_retries ?? s.MaxRetries ?? 3
      }))
    }
    expandedStep.value = formData.value.steps.length > 0 ? 0 : null
    isDialogOpen.value = true
  } catch (error) {
    toast.error('Failed to load flow details')
  }
}

function addStep() {
  const newOrder = formData.value.steps.length + 1
  formData.value.steps.push({
    ...defaultStep,
    step_name: `step_${newOrder}`,
    step_order: newOrder,
    message: '',
    store_as: ''
  })
  expandedStep.value = formData.value.steps.length - 1
}

// Completion webhook header helpers
function addCompletionHeader() {
  const headerNum = Object.keys(formData.value.completion_config.headers).length + 1
  formData.value.completion_config.headers[`Header-${headerNum}`] = ''
}

function updateCompletionHeaderKey(oldKey: string, newKey: string) {
  if (oldKey === newKey) return
  const value = formData.value.completion_config.headers[oldKey]
  delete formData.value.completion_config.headers[oldKey]
  formData.value.completion_config.headers[newKey] = value
}

function removeCompletionHeader(key: string) {
  delete formData.value.completion_config.headers[key]
}

// Step API header helpers
function addStepHeader(stepIndex: number) {
  const step = formData.value.steps[stepIndex]
  if (!step.api_config.headers) {
    step.api_config.headers = {}
  }
  const headerNum = Object.keys(step.api_config.headers).length + 1
  step.api_config.headers[`Header-${headerNum}`] = ''
}

function updateStepHeaderKey(stepIndex: number, oldKey: string, newKey: string) {
  if (oldKey === newKey) return
  const step = formData.value.steps[stepIndex]
  const value = step.api_config.headers[oldKey]
  delete step.api_config.headers[oldKey]
  step.api_config.headers[newKey] = value
}

function removeStepHeader(stepIndex: number, key: string) {
  delete formData.value.steps[stepIndex].api_config.headers[key]
}

function removeStep(index: number) {
  formData.value.steps.splice(index, 1)
  // Reorder remaining steps
  formData.value.steps.forEach((step, idx) => {
    step.step_order = idx + 1
    if (step.step_name.startsWith('step_')) {
      step.step_name = `step_${idx + 1}`
    }
  })
  if (expandedStep.value === index) {
    expandedStep.value = null
  } else if (expandedStep.value !== null && expandedStep.value > index) {
    expandedStep.value--
  }
}

function moveStep(index: number, direction: 'up' | 'down') {
  const newIndex = direction === 'up' ? index - 1 : index + 1
  if (newIndex < 0 || newIndex >= formData.value.steps.length) return

  const steps = formData.value.steps
  const temp = steps[index]
  steps[index] = steps[newIndex]
  steps[newIndex] = temp

  // Update step_order
  steps.forEach((step, idx) => {
    step.step_order = idx + 1
  })

  if (expandedStep.value === index) {
    expandedStep.value = newIndex
  } else if (expandedStep.value === newIndex) {
    expandedStep.value = index
  }
}

async function saveFlow() {
  if (!formData.value.name.trim()) {
    toast.error('Please enter a flow name')
    return
  }
  if (formData.value.steps.length === 0) {
    toast.error('Please add at least one step')
    return
  }

  isSubmitting.value = true
  try {
    const data = {
      name: formData.value.name,
      description: formData.value.description,
      trigger_keywords: formData.value.trigger_keywords.split(',').map(k => k.trim()).filter(Boolean),
      initial_message: formData.value.initial_message,
      completion_message: formData.value.completion_message,
      on_complete_action: formData.value.on_complete_action,
      completion_config: formData.value.on_complete_action === 'webhook' ? formData.value.completion_config : {},
      enabled: formData.value.enabled,
      steps: formData.value.steps.map((step, idx) => ({
        ...step,
        step_order: idx + 1,
        step_name: step.step_name || `step_${idx + 1}`
      }))
    }

    if (editingFlow.value) {
      await chatbotService.updateFlow(editingFlow.value.id, data)
      toast.success('Flow updated')
    } else {
      await chatbotService.createFlow(data)
      toast.success('Flow created')
    }

    isDialogOpen.value = false
    await fetchFlows()
  } catch (error) {
    toast.error('Failed to save flow')
  } finally {
    isSubmitting.value = false
  }
}

async function toggleFlow(flow: ChatbotFlow) {
  try {
    await chatbotService.updateFlow(flow.id, { enabled: !flow.enabled })
    flow.enabled = !flow.enabled
    toast.success(flow.enabled ? 'Flow enabled' : 'Flow disabled')
  } catch (error) {
    toast.error('Failed to toggle flow')
  }
}

function openDeleteDialog(flow: ChatbotFlow) {
  flowToDelete.value = flow
  deleteDialogOpen.value = true
}

async function confirmDeleteFlow() {
  if (!flowToDelete.value) return

  try {
    await chatbotService.deleteFlow(flowToDelete.value.id)
    toast.success('Flow deleted')
    deleteDialogOpen.value = false
    flowToDelete.value = null
    await fetchFlows()
  } catch (error) {
    toast.error('Failed to delete flow')
  }
}

const inputTypes = [
  { value: 'none', label: 'No input required' },
  { value: 'text', label: 'Text' },
  { value: 'number', label: 'Number' },
  { value: 'email', label: 'Email' },
  { value: 'phone', label: 'Phone number' },
  { value: 'date', label: 'Date' },
  { value: 'select', label: 'Selection (buttons)' }
]

const messageTypes = [
  { value: 'text', label: 'Static Text' },
  { value: 'buttons', label: 'Text with Buttons' },
  { value: 'api_fetch', label: 'Fetch from API' },
  { value: 'whatsapp_flow', label: 'WhatsApp Flow' }
]

const httpMethods = ['GET', 'POST', 'PUT', 'PATCH']

function addButton(step: FlowStep) {
  if (step.buttons.length >= 10) {
    toast.error('WhatsApp allows maximum 10 options')
    return
  }
  step.buttons.push({ id: `btn_${step.buttons.length + 1}`, title: '' })
}

function removeButton(step: FlowStep, index: number) {
  step.buttons.splice(index, 1)
}
</script>

<template>
  <div class="flex flex-col h-full">
    <!-- Header -->
    <header class="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div class="flex h-16 items-center justify-between px-6">
        <div class="flex items-center gap-4">
          <RouterLink to="/chatbot">
            <Button variant="ghost" size="icon">
              <ArrowLeft class="h-5 w-5" />
            </Button>
          </RouterLink>
          <div>
            <h1 class="text-xl font-semibold">Conversation Flows</h1>
            <p class="text-sm text-muted-foreground">Design multi-step conversation experiences</p>
          </div>
        </div>
        <Dialog v-model:open="isDialogOpen">
          <DialogTrigger as-child>
            <Button @click="openCreateDialog">
              <Plus class="h-4 w-4 mr-2" />
              Create Flow
            </Button>
          </DialogTrigger>
          <DialogContent class="max-w-4xl max-h-[90vh] flex flex-col">
            <DialogHeader class="flex-shrink-0">
              <DialogTitle>{{ editingFlow ? 'Edit' : 'Create' }} Conversation Flow</DialogTitle>
              <DialogDescription>
                Design a multi-step conversation to collect information from users.
              </DialogDescription>
            </DialogHeader>
            <div class="flex-1 overflow-y-auto pr-4 min-h-0">
              <div class="grid gap-6 py-4">
                <!-- Basic Info -->
                <div class="grid gap-4">
                  <div class="grid grid-cols-2 gap-4">
                    <div class="space-y-2">
                      <Label for="name">Flow Name *</Label>
                      <Input
                        id="name"
                        v-model="formData.name"
                        placeholder="Customer Support Flow"
                      />
                    </div>
                    <div class="space-y-2">
                      <Label for="trigger_keywords">Trigger Keywords (comma-separated)</Label>
                      <Input
                        id="trigger_keywords"
                        v-model="formData.trigger_keywords"
                        placeholder="help, support, order"
                      />
                    </div>
                  </div>
                  <div class="space-y-2">
                    <Label for="description">Description</Label>
                    <Input
                      id="description"
                      v-model="formData.description"
                      placeholder="Handles customer support requests"
                    />
                  </div>
                  <div class="grid grid-cols-2 gap-4">
                    <div class="space-y-2">
                      <Label for="initial_message">Initial Message</Label>
                      <Textarea
                        id="initial_message"
                        v-model="formData.initial_message"
                        placeholder="Hi! Let me help you with that."
                        :rows="2"
                      />
                    </div>
                    <div class="space-y-2">
                      <Label for="completion_message">Completion Message</Label>
                      <Textarea
                        id="completion_message"
                        v-model="formData.completion_message"
                        placeholder="Thank you! We'll get back to you soon."
                        :rows="2"
                      />
                    </div>
                  </div>

                  <!-- On Complete Action -->
                  <div class="space-y-4">
                    <div class="space-y-2">
                      <Label>On Flow Completion</Label>
                      <Select v-model="formData.on_complete_action">
                        <SelectTrigger>
                          <SelectValue placeholder="Select action" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="none">No action</SelectItem>
                          <SelectItem value="webhook">Send data to API/Webhook</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>

                    <!-- Webhook Configuration -->
                    <div v-if="formData.on_complete_action === 'webhook'" class="space-y-4 p-4 border rounded-lg bg-muted/10">
                      <div class="flex items-center gap-2 mb-2">
                        <Badge variant="outline">Webhook Configuration</Badge>
                      </div>

                      <div class="grid grid-cols-4 gap-4">
                        <div class="space-y-2">
                          <Label>Method</Label>
                          <Select v-model="formData.completion_config.method">
                            <SelectTrigger>
                              <SelectValue placeholder="Method" />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem v-for="method in httpMethods" :key="method" :value="method">
                                {{ method }}
                              </SelectItem>
                            </SelectContent>
                          </Select>
                        </div>
                        <div class="col-span-3 space-y-2">
                          <Label>Webhook URL *</Label>
                          <Input
                            v-model="formData.completion_config.url"
                            placeholder="https://api.example.com/webhook"
                          />
                        </div>
                      </div>

                      <!-- Headers -->
                      <div class="space-y-2">
                        <div class="flex items-center justify-between">
                          <Label>Headers (optional)</Label>
                          <Button variant="outline" size="sm" @click="addCompletionHeader">
                            <Plus class="h-3 w-3 mr-1" />
                            Add Header
                          </Button>
                        </div>
                        <div v-if="Object.keys(formData.completion_config.headers).length > 0" class="space-y-2">
                          <div
                            v-for="(value, key) in formData.completion_config.headers"
                            :key="key"
                            class="flex items-center gap-2"
                          >
                            <Input
                              :model-value="key"
                              placeholder="Header name"
                              class="flex-1"
                              @update:model-value="updateCompletionHeaderKey(key as string, $event)"
                            />
                            <Input
                              v-model="formData.completion_config.headers[key as string]"
                              placeholder="Header value"
                              class="flex-1"
                            />
                            <Button variant="ghost" size="icon" @click="removeCompletionHeader(key as string)">
                              <Trash2 class="h-4 w-4 text-destructive" />
                            </Button>
                          </div>
                        </div>
                        <p class="text-xs text-muted-foreground">
                          Add custom headers like Authorization, API keys, etc. Use {{variable}} for dynamic values.
                        </p>
                      </div>

                      <div class="space-y-2">
                        <Label>Custom Request Body (optional)</Label>
                        <Textarea
                          v-model="formData.completion_config.body"
                          placeholder='Leave empty for default payload, or enter custom JSON like: {"name": "{{name}}", "phone": "{{phone_number}}"}'
                          :rows="3"
                        />
                        <p class="text-xs text-muted-foreground">
                          Default payload includes: flow_id, flow_name, session_id, phone_number, contact_id, contact_name, session_data, completed_at
                        </p>
                      </div>
                    </div>
                  </div>
                </div>

                <!-- Steps -->
                <div class="space-y-4">
                  <div class="flex items-center justify-between">
                    <Label class="text-base font-medium">Flow Steps</Label>
                    <Button variant="outline" size="sm" @click="addStep">
                      <Plus class="h-4 w-4 mr-1" />
                      Add Step
                    </Button>
                  </div>

                  <div v-if="formData.steps.length === 0" class="text-center py-8 border rounded-lg border-dashed">
                    <p class="text-muted-foreground mb-2">No steps yet</p>
                    <Button variant="outline" size="sm" @click="addStep">
                      <Plus class="h-4 w-4 mr-1" />
                      Add First Step
                    </Button>
                  </div>

                  <div v-else class="space-y-2">
                    <div
                      v-for="(step, index) in formData.steps"
                      :key="index"
                      class="border rounded-lg"
                    >
                      <!-- Step Header -->
                      <div
                        class="flex items-center gap-2 p-3 cursor-pointer hover:bg-muted/50"
                        @click="expandedStep = expandedStep === index ? null : index"
                      >
                        <GripVertical class="h-4 w-4 text-muted-foreground" />
                        <Badge variant="outline" class="font-mono">{{ index + 1 }}</Badge>
                        <span class="flex-1 font-medium">{{ step.step_name || `Step ${index + 1}` }}</span>
                        <span class="text-sm text-muted-foreground truncate max-w-[200px]">
                          {{ step.message || 'No message' }}
                        </span>
                        <div class="flex items-center gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            class="h-7 w-7"
                            :disabled="index === 0"
                            @click.stop="moveStep(index, 'up')"
                          >
                            <ChevronUp class="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            class="h-7 w-7"
                            :disabled="index === formData.steps.length - 1"
                            @click.stop="moveStep(index, 'down')"
                          >
                            <ChevronDown class="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            class="h-7 w-7 text-destructive"
                            @click.stop="removeStep(index)"
                          >
                            <Trash2 class="h-4 w-4" />
                          </Button>
                        </div>
                      </div>

                      <!-- Step Content (Expanded) -->
                      <div v-if="expandedStep === index" class="p-4 border-t bg-muted/20 space-y-4">
                        <div class="grid grid-cols-2 gap-4">
                          <div class="space-y-2">
                            <Label>Step Name</Label>
                            <Input v-model="step.step_name" placeholder="step_1" />
                          </div>
                          <div class="space-y-2">
                            <Label>Store Response As</Label>
                            <Input v-model="step.store_as" placeholder="customer_name" />
                            <p class="text-xs text-muted-foreground">Variable name to store the user's response</p>
                          </div>
                        </div>

                        <!-- Message Type Selector -->
                        <div class="space-y-2">
                          <Label>Message Source</Label>
                          <Select v-model="step.message_type">
                            <SelectTrigger>
                              <SelectValue placeholder="Select message source" />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem v-for="type in messageTypes" :key="type.value" :value="type.value">
                                {{ type.label }}
                              </SelectItem>
                            </SelectContent>
                          </Select>
                        </div>

                        <!-- Static Message (for text type) -->
                        <div v-if="step.message_type !== 'api_fetch'" class="space-y-2">
                          <Label>Message to Send *</Label>
                          <Textarea
                            v-model="step.message"
                            placeholder="What is your name?"
                            :rows="2"
                          />
                        </div>

                        <!-- Buttons Configuration (for buttons type) -->
                        <div v-if="step.message_type === 'buttons'" class="space-y-4 p-4 border rounded-lg bg-muted/10">
                          <div class="flex items-center justify-between mb-2">
                            <div class="flex items-center gap-2">
                              <Badge variant="outline">Button Options</Badge>
                              <span class="text-xs text-muted-foreground">
                                ({{ step.buttons.length }}/10) {{ step.buttons.length <= 3 ? '- Shows as buttons' : '- Shows as list' }}
                              </span>
                            </div>
                            <Button variant="outline" size="sm" @click="addButton(step)" :disabled="step.buttons.length >= 10">
                              <Plus class="h-4 w-4 mr-1" />
                              Add Option
                            </Button>
                          </div>

                          <div v-if="step.buttons.length === 0" class="text-center py-4 text-muted-foreground text-sm">
                            No buttons added yet. Click "Add Option" to add buttons.
                          </div>

                          <div v-else class="space-y-2">
                            <div v-for="(button, btnIndex) in step.buttons" :key="btnIndex" class="flex items-center gap-2">
                              <Input
                                v-model="button.id"
                                placeholder="btn_id"
                                class="w-24"
                              />
                              <Input
                                v-model="button.title"
                                :placeholder="`Option ${btnIndex + 1}`"
                                class="flex-1"
                                :maxlength="step.buttons.length <= 3 ? 20 : 24"
                              />
                              <Button variant="ghost" size="icon" class="h-8 w-8 text-destructive" @click="removeButton(step, btnIndex)">
                                <Trash2 class="h-4 w-4" />
                              </Button>
                            </div>
                          </div>
                          <p class="text-xs text-muted-foreground">
                            Button titles: max 20 chars (buttons) or 24 chars (list). IDs are used for conditional routing.
                          </p>
                        </div>

                        <!-- API Configuration (for api_fetch type) -->
                        <div v-if="step.message_type === 'api_fetch'" class="space-y-4 p-4 border rounded-lg bg-muted/10">
                          <div class="flex items-center gap-2 mb-2">
                            <Badge variant="outline">API Configuration</Badge>
                          </div>

                          <div class="grid grid-cols-4 gap-4">
                            <div class="space-y-2">
                              <Label>Method</Label>
                              <Select v-model="step.api_config.method">
                                <SelectTrigger>
                                  <SelectValue placeholder="Method" />
                                </SelectTrigger>
                                <SelectContent>
                                  <SelectItem v-for="method in httpMethods" :key="method" :value="method">
                                    {{ method }}
                                  </SelectItem>
                                </SelectContent>
                              </Select>
                            </div>
                            <div class="col-span-3 space-y-2">
                              <Label>API URL *</Label>
                              <Input
                                v-model="step.api_config.url"
                                placeholder="https://api.example.com/data/{{customer_id}}"
                              />
                              <p class="text-xs text-muted-foreground">Use {{variable}} to include session data</p>
                            </div>
                          </div>

                          <!-- Headers -->
                          <div class="space-y-2">
                            <div class="flex items-center justify-between">
                              <Label>Headers (optional)</Label>
                              <Button variant="outline" size="sm" @click="addStepHeader(index)">
                                <Plus class="h-3 w-3 mr-1" />
                                Add Header
                              </Button>
                            </div>
                            <div v-if="Object.keys(step.api_config.headers).length > 0" class="space-y-2">
                              <div
                                v-for="(value, key) in step.api_config.headers"
                                :key="key"
                                class="flex items-center gap-2"
                              >
                                <Input
                                  :model-value="key"
                                  placeholder="Header name"
                                  class="flex-1"
                                  @update:model-value="updateStepHeaderKey(index, key as string, $event)"
                                />
                                <Input
                                  v-model="step.api_config.headers[key as string]"
                                  placeholder="Header value"
                                  class="flex-1"
                                />
                                <Button variant="ghost" size="icon" @click="removeStepHeader(index, key as string)">
                                  <Trash2 class="h-4 w-4 text-destructive" />
                                </Button>
                              </div>
                            </div>
                            <p class="text-xs text-muted-foreground">
                              Add custom headers like Authorization, API keys. Use {{variable}} for dynamic values.
                            </p>
                          </div>

                          <div v-if="step.api_config.method !== 'GET'" class="space-y-2">
                            <Label>Request Body (JSON)</Label>
                            <Textarea
                              v-model="step.api_config.body"
                              placeholder='{"customer_id": "{{customer_id}}", "name": "{{name}}"}'
                              :rows="3"
                            />
                          </div>

                          <div class="grid grid-cols-2 gap-4">
                            <div class="space-y-2">
                              <Label>Response Path</Label>
                              <Input
                                v-model="step.api_config.response_path"
                                placeholder="data.message"
                              />
                              <p class="text-xs text-muted-foreground">Dot notation path to extract message (e.g., data.message)</p>
                            </div>
                            <div class="space-y-2">
                              <Label>Fallback Message</Label>
                              <Input
                                v-model="step.api_config.fallback_message"
                                placeholder="Sorry, we couldn't fetch your data."
                              />
                              <p class="text-xs text-muted-foreground">Sent if API call fails</p>
                            </div>
                          </div>
                        </div>

                        <!-- WhatsApp Flow Configuration -->
                        <div v-if="step.message_type === 'whatsapp_flow'" class="space-y-4 p-4 border rounded-lg bg-muted/10">
                          <div class="flex items-center gap-2 mb-2">
                            <Badge variant="outline">WhatsApp Flow</Badge>
                          </div>

                          <div class="space-y-2">
                            <Label>Select WhatsApp Flow *</Label>
                            <Select v-model="step.input_config.whatsapp_flow_id">
                              <SelectTrigger>
                                <SelectValue :placeholder="whatsappFlows.length === 0 ? 'No published flows available' : 'Select a published flow'" />
                              </SelectTrigger>
                              <SelectContent>
                                <SelectItem v-for="wf in whatsappFlows" :key="wf.id" :value="wf.meta_flow_id">
                                  {{ wf.name }}
                                </SelectItem>
                              </SelectContent>
                            </Select>
                            <p class="text-xs text-muted-foreground">
                              Only published WhatsApp Flows are available. The flow will be sent as an interactive message.
                            </p>
                          </div>

                          <div class="space-y-2">
                            <Label>Flow Header Text (optional)</Label>
                            <Input
                              v-model="step.input_config.flow_header"
                              placeholder="Complete the form below"
                            />
                          </div>

                          <div class="space-y-2">
                            <Label>Flow Body Text</Label>
                            <Textarea
                              v-model="step.message"
                              placeholder="Please fill out this form to continue."
                              :rows="2"
                            />
                          </div>

                          <div class="space-y-2">
                            <Label>Flow Button Text</Label>
                            <Input
                              v-model="step.input_config.flow_cta"
                              placeholder="Open Form"
                              maxlength="20"
                            />
                            <p class="text-xs text-muted-foreground">Max 20 characters</p>
                          </div>
                        </div>

                        <div class="grid grid-cols-2 gap-4">
                          <div class="space-y-2">
                            <Label>Expected Input Type</Label>
                            <Select v-model="step.input_type">
                              <SelectTrigger>
                                <SelectValue placeholder="Select input type" />
                              </SelectTrigger>
                              <SelectContent>
                                <SelectItem v-for="type in inputTypes" :key="type.value" :value="type.value">
                                  {{ type.label }}
                                </SelectItem>
                              </SelectContent>
                            </Select>
                          </div>
                          <div class="space-y-2">
                            <Label>Validation Regex (optional)</Label>
                            <Input v-model="step.validation_regex" placeholder="^[A-Za-z ]+$" />
                          </div>
                        </div>

                        <div v-if="step.input_type === 'select'" class="space-y-2">
                          <Label>Button Options (one per line)</Label>
                          <Textarea
                            :model-value="(step.input_config.options || []).join('\n')"
                            @update:model-value="step.input_config = { ...step.input_config, options: ($event as string).split('\n').filter(Boolean) }"
                            placeholder="Option 1&#10;Option 2&#10;Option 3"
                            :rows="3"
                          />
                        </div>

                        <div class="space-y-2">
                          <Label>Validation Error Message</Label>
                          <Input v-model="step.validation_error" placeholder="Invalid input. Please try again." />
                        </div>

                        <div class="flex items-center gap-4">
                          <div class="flex items-center gap-2">
                            <Switch
                              :id="`retry-${index}`"
                              :checked="step.retry_on_invalid"
                              @update:checked="step.retry_on_invalid = $event"
                            />
                            <Label :for="`retry-${index}`">Retry on invalid input</Label>
                          </div>
                          <div v-if="step.retry_on_invalid" class="flex items-center gap-2">
                            <Label>Max retries:</Label>
                            <Input
                              v-model.number="step.max_retries"
                              type="number"
                              min="1"
                              max="10"
                              class="w-20"
                            />
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <DialogFooter class="flex-shrink-0 border-t pt-4">
              <Button variant="outline" @click="isDialogOpen = false">Cancel</Button>
              <Button @click="saveFlow" :disabled="isSubmitting">
                {{ editingFlow ? 'Update' : 'Create' }} Flow
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </header>

    <!-- Flows List -->
    <ScrollArea class="flex-1">
      <div class="p-6 grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        <!-- Loading Skeleton -->
        <template v-if="isLoading">
          <Card v-for="i in 6" :key="i" class="flex flex-col">
            <CardHeader>
              <div class="flex items-start justify-between">
                <div class="flex items-center gap-3">
                  <Skeleton class="h-10 w-10 rounded-lg" />
                  <div>
                    <Skeleton class="h-5 w-32 mb-2" />
                    <Skeleton class="h-5 w-16" />
                  </div>
                </div>
              </div>
            </CardHeader>
            <CardContent class="flex-1">
              <Skeleton class="h-4 w-full mb-3" />
              <div class="flex gap-1 mb-3">
                <Skeleton class="h-5 w-14" />
                <Skeleton class="h-5 w-16" />
              </div>
              <Skeleton class="h-4 w-20" />
            </CardContent>
            <div class="p-4 pt-0 flex items-center justify-between border-t mt-auto">
              <div class="flex gap-2">
                <Skeleton class="h-8 w-8 rounded" />
                <Skeleton class="h-8 w-8 rounded" />
              </div>
              <Skeleton class="h-8 w-20" />
            </div>
          </Card>
        </template>

        <template v-else>
        <Card v-for="flow in flows" :key="flow.id" class="flex flex-col">
          <CardHeader>
            <div class="flex items-start justify-between">
              <div class="flex items-center gap-3">
                <div class="h-10 w-10 rounded-lg bg-purple-100 dark:bg-purple-900 flex items-center justify-center">
                  <Workflow class="h-5 w-5 text-purple-600 dark:text-purple-400" />
                </div>
                <div>
                  <CardTitle class="text-base">{{ flow.name }}</CardTitle>
                  <Badge :variant="flow.enabled ? 'default' : 'secondary'" class="mt-1">
                    {{ flow.enabled ? 'Active' : 'Inactive' }}
                  </Badge>
                </div>
              </div>
            </div>
          </CardHeader>
          <CardContent class="flex-1">
            <p class="text-sm text-muted-foreground mb-3">{{ flow.description || 'No description' }}</p>
            <div class="flex flex-wrap gap-1 mb-3" v-if="flow.trigger_keywords?.length">
              <Badge v-for="keyword in flow.trigger_keywords" :key="keyword" variant="outline">
                {{ keyword }}
              </Badge>
            </div>
            <p class="text-xs text-muted-foreground">{{ flow.steps_count }} steps</p>
          </CardContent>
          <div class="p-4 pt-0 flex items-center justify-between border-t mt-auto">
            <div class="flex gap-2">
              <Tooltip>
                <TooltipTrigger as-child>
                  <Button variant="ghost" size="icon" @click="openEditDialog(flow)">
                    <Pencil class="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Edit flow</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger as-child>
                  <Button variant="ghost" size="icon" @click="openDeleteDialog(flow)">
                    <Trash2 class="h-4 w-4 text-destructive" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Delete flow</TooltipContent>
              </Tooltip>
            </div>
            <Button
              :variant="flow.enabled ? 'outline' : 'default'"
              size="sm"
              @click="toggleFlow(flow)"
            >
              <component :is="flow.enabled ? Pause : Play" class="h-4 w-4 mr-1" />
              {{ flow.enabled ? 'Disable' : 'Enable' }}
            </Button>
          </div>
        </Card>

        <Card v-if="flows.length === 0" class="col-span-full">
          <CardContent class="py-12 text-center text-muted-foreground">
            <Workflow class="h-12 w-12 mx-auto mb-4 opacity-50" />
            <p class="text-lg font-medium">No conversation flows yet</p>
            <p class="text-sm mb-4">Create your first flow to automate conversations.</p>
            <Button @click="openCreateDialog">
              <Plus class="h-4 w-4 mr-2" />
              Create Flow
            </Button>
          </CardContent>
        </Card>
        </template>
      </div>
    </ScrollArea>

    <!-- Delete Confirmation Dialog -->
    <AlertDialog v-model:open="deleteDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete Flow</AlertDialogTitle>
          <AlertDialogDescription>
            Are you sure you want to delete "{{ flowToDelete?.name }}"? This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction @click="confirmDeleteFlow">Delete</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
