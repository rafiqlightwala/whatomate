import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useContactsStore, type Contact, type Message } from '@/stores/contacts'
import { contactsService } from '@/services/api'
import { wsService } from './websocket'

vi.mock('@/services/api', () => ({
  contactsService: { list: vi.fn(), markRead: vi.fn() },
  messagesService: {}
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: { id: 'agent' }, userSettings: { new_message_alerts: false } })
}))
vi.mock('@/stores/transfers', () => ({ useTransfersStore: () => ({}) }))
vi.mock('@/stores/calling', () => ({ useCallingStore: () => ({}) }))
vi.mock('@/stores/notes', () => ({ useNotesStore: () => ({}) }))
vi.mock('@/router', () => ({ default: {} }))
vi.mock('vue-sonner', () => ({ toast: {} }))

class MockSocket {
  static OPEN = 1
  static CONNECTING = 0
  static latest: MockSocket
  readyState = MockSocket.OPEN
  onmessage: ((event: { data: string }) => void) | null = null
  close() {}
  constructor() { MockSocket.latest = this }
}

function contact(id = 'contact-1'): Contact {
  return {
    id, phone_number: '123', name: 'Test', status: 'active', tags: [],
    metadata: {}, unread_count: 0, created_at: '', updated_at: ''
  }
}

function incoming(overrides: Partial<Message> = {}): Message {
  return {
    id: 'message-1', contact_id: 'contact-1', direction: 'incoming',
    message_type: 'text', content: { body: 'سلام Investify' }, status: 'delivered',
    created_at: '2026-09-30T12:00:00Z', updated_at: '2026-09-30T12:00:00Z',
    ...overrides
  }
}

function receive(payload: unknown, type = 'new_message') {
  MockSocket.latest.onmessage!({ data: JSON.stringify({ type, payload }) })
}

beforeEach(async () => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  setActivePinia(createPinia())
  vi.stubGlobal('WebSocket', MockSocket)
  vi.stubGlobal('window', {
    location: { protocol: 'http:', host: 'localhost' },
    addEventListener: vi.fn(), removeEventListener: vi.fn()
  })
  vi.stubGlobal('document', {
    visibilityState: 'visible', hasFocus: () => true,
    addEventListener: vi.fn(), removeEventListener: vi.fn()
  })
  vi.mocked(contactsService.list).mockResolvedValue({ data: { contacts: [], total: 0 } } as never)
  vi.mocked(contactsService.markRead).mockResolvedValue({} as never)
  await wsService.connect(async () => 'test-token')
})

afterEach(() => {
  wsService.disconnect()
  vi.clearAllTimers()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('merged live inbox behavior', () => {
  it('updates the preview and unread count once without refetching loaded contacts', async () => {
    const store = useContactsStore()
    store.contacts = [contact()]
    receive(incoming())
    expect(store.contacts[0].last_message_preview).toBe('سلام Investify')
    expect(store.contacts[0].unread_count).toBe(1)
    expect(store.contacts[0].service_window_open).toBe(true)
    await vi.advanceTimersByTimeAsync(3000)
    expect(contactsService.list).not.toHaveBeenCalled()
  })

  it('preserves media previews without counting bot-read messages as unread', () => {
    const store = useContactsStore()
    store.contacts = [contact()]
    receive(incoming({ message_type: 'document', status: 'read', content: {} }))
    expect(store.contacts[0].last_message_preview).toBe('[Document]')
    expect(store.contacts[0].unread_count).toBe(0)
    expect(contactsService.markRead).not.toHaveBeenCalled()
  })

  it('normalizes IDs and marks the open conversation read without a list request', async () => {
    const store = useContactsStore()
    store.contacts = [contact()]
    store.currentContact = contact()
    receive({ ...incoming(), id: { ID: 'message-1' }, contact_id: { id: 'contact-1' } })
    await Promise.resolve()
    expect(store.messages).toHaveLength(1)
    expect(store.messages[0].id).toBe('message-1')
    expect(store.messages[0].contact_id).toBe('contact-1')
    expect(contactsService.markRead).toHaveBeenCalledWith('contact-1')
    expect(store.contacts[0].unread_count).toBe(0)
    expect(store.currentContact.unread_count).toBe(0)
    await vi.advanceTimersByTimeAsync(3000)
    expect(contactsService.list).not.toHaveBeenCalled()
  })

  it('keeps unread messages unread when the open chat is in a hidden tab', () => {
    const store = useContactsStore()
    store.contacts = [contact()]
    store.currentContact = store.contacts[0]
    vi.stubGlobal('document', {
      visibilityState: 'hidden', hasFocus: () => false, removeEventListener: vi.fn()
    })
    receive(incoming())
    expect(store.messages).toHaveLength(1)
    expect(store.contacts[0].unread_count).toBe(1)
    expect(contactsService.markRead).not.toHaveBeenCalled()
  })

  it('coalesces new-contact refreshes and keeps the active search filter', async () => {
    const store = useContactsStore()
    store.searchQuery = 'Investify'
    await vi.advanceTimersByTimeAsync(300)
    vi.mocked(contactsService.list).mockClear()
    receive(incoming())
    receive(incoming({ id: 'message-2', contact_id: 'contact-2' }))
    expect(contactsService.list).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(3000)
    expect(contactsService.list).toHaveBeenCalledTimes(1)
    expect(contactsService.list).toHaveBeenCalledWith(expect.objectContaining({ search: 'Investify' }))
  })

  it.each(['status_update', 'message_status'])('accepts %s delivery updates', (type) => {
    const store = useContactsStore()
    store.messages = [incoming({ direction: 'outgoing', status: 'sent' })]
    receive({ message_id: 'message-1', status: 'delivered' }, type)
    expect(store.messages[0].status).toBe('delivered')
  })
})
