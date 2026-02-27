<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useCallingStore } from '@/stores/calling'
import { Button } from '@/components/ui/button'
import { Phone, PhoneOff, PhoneIncoming, Mic, MicOff } from 'lucide-vue-next'
import { toast } from 'vue-sonner'

const { t } = useI18n()
const store = useCallingStore()
const acceptingId = ref<string | null>(null)

const formattedDuration = computed(() => {
  const m = Math.floor(store.callDuration / 60)
  const s = store.callDuration % 60
  return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
})

const displayName = computed(() => {
  if (store.isOutgoingCall) {
    return store.outgoingContactName || store.outgoingContactPhone || 'Unknown'
  }
  return store.activeTransfer?.contact?.profile_name || store.activeTransfer?.caller_phone || 'Unknown'
})

const statusText = computed(() => {
  if (store.isOutgoingCall) {
    switch (store.outgoingCallStatus) {
      case 'initiating': return `${t('outgoingCalls.initiating')}...`
      case 'ringing': return `${t('outgoingCalls.ringing')}...`
      case 'answered': return t('outgoingCalls.answered')
      default: return ''
    }
  }
  return t('callTransfers.callConnected')
})

const showPanel = computed(() => store.isOnCall || store.waitingTransfers.length > 0)

async function handleAccept(id: string) {
  acceptingId.value = id
  try {
    await store.acceptTransfer(id)
    toast.success(t('callTransfers.callConnected'))
  } catch (err: any) {
    toast.error(t('callTransfers.acceptFailed'), {
      description: err.message || ''
    })
  } finally {
    acceptingId.value = null
  }
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="showPanel"
      class="fixed bottom-6 right-6 z-50 flex flex-col gap-3 items-end"
    >
      <!-- Waiting incoming transfers — each as its own call card -->
      <div
        v-for="transfer in store.waitingTransfers.slice(0, 3)"
        :key="transfer.id"
        class="bg-zinc-900 border border-green-700/50 rounded-xl shadow-2xl p-4 min-w-[260px]"
      >
        <div class="flex items-center gap-3 mb-3">
          <div class="w-8 h-8 rounded-full bg-green-600/20 flex items-center justify-center">
            <PhoneIncoming class="h-4 w-4 text-green-400 animate-pulse" />
          </div>
          <div class="min-w-0 flex-1">
            <p class="text-sm font-medium text-zinc-100 truncate">
              {{ transfer.contact?.profile_name || transfer.caller_phone }}
            </p>
            <p class="text-xs text-green-400">
              {{ t('callTransfers.incomingTransfer') }}
              <span v-if="transfer.team?.name" class="text-zinc-500"> · {{ transfer.team.name }}</span>
            </p>
          </div>
        </div>

        <div class="flex items-center justify-center gap-4">
          <Button
            size="sm"
            class="h-10 w-10 rounded-full p-0 bg-green-600 hover:bg-green-500"
            :disabled="acceptingId === transfer.id"
            @click="handleAccept(transfer.id)"
          >
            <Phone class="h-4 w-4 text-white" />
          </Button>
        </div>
      </div>

      <!-- Active call panel -->
      <div
        v-if="store.isOnCall"
        class="bg-zinc-900 border border-zinc-700 rounded-xl shadow-2xl p-4 min-w-[260px]"
      >
        <div class="flex items-center gap-3 mb-3">
          <div class="w-8 h-8 rounded-full bg-green-600/20 flex items-center justify-center">
            <Phone class="h-4 w-4 text-green-400" />
          </div>
          <div>
            <p class="text-sm font-medium text-zinc-100">
              {{ displayName }}
            </p>
            <p class="text-xs text-zinc-400">{{ statusText }}</p>
          </div>
        </div>

        <div class="text-center mb-3">
          <span class="text-2xl font-mono text-zinc-200">{{ formattedDuration }}</span>
        </div>

        <div class="flex items-center justify-center gap-3">
          <Button
            size="sm"
            variant="outline"
            class="h-10 w-10 rounded-full p-0"
            :class="store.isMuted ? 'bg-red-900/30 border-red-700' : 'border-zinc-600'"
            @click="store.toggleMute()"
          >
            <MicOff v-if="store.isMuted" class="h-4 w-4 text-red-400" />
            <Mic v-else class="h-4 w-4 text-zinc-300" />
          </Button>

          <Button
            size="sm"
            class="h-10 w-10 rounded-full p-0 bg-red-600 hover:bg-red-500"
            @click="store.endCall()"
          >
            <PhoneOff class="h-4 w-4 text-white" />
          </Button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
