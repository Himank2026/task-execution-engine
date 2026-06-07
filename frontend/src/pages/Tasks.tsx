import { useEffect, useState } from 'react'
import {
  ActionIcon,
  Badge,
  Button,
  Card,
  Divider,
  Drawer,
  Group,
  NumberInput,
  Pagination,
  Select,
  Stack,
  Switch,
  Table,
  Text,
  Title,
  Tooltip,
} from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { notifications } from '@mantine/notifications'
import { IconX } from '@tabler/icons-react'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'

import { api, CLIENTS, clientColor, type Task, type TaskPage } from '../api'

dayjs.extend(relativeTime) // enables "3 seconds ago" formatting

const TASK_TYPES = ['send_email', 'send_sms', 'generate_report', 'resize_image']

// Filter dropdown options ("all" = no filter).
const CLIENT_OPTIONS = [{ value: 'all', label: 'All clients' }, ...CLIENTS.map((c) => ({ value: c, label: c }))]
const TYPE_OPTIONS = [{ value: 'all', label: 'All types' }, ...TASK_TYPES.map((t) => ({ value: t, label: t }))]
const STATUS_OPTIONS = [
  { value: 'all', label: 'All statuses' },
  { value: 'pending', label: 'pending' },
  { value: 'running', label: 'running' },
  { value: 'completed', label: 'completed' },
  { value: 'failed', label: 'failed' },
  { value: 'cancelled', label: 'cancelled' },
]

// One labelled line in the task detail drawer.
function DetailRow({ label, value }: { label: string; value: string | number }) {
  return (
    <Group justify="space-between" wrap="nowrap">
      <Text size="sm" c="dimmed">
        {label}
      </Text>
      <Text size="sm">{value}</Text>
    </Group>
  )
}

const fmtTime = (ts: string) => dayjs(ts).format('MMM D, HH:mm:ss')
const secsBetween = (from: string, to: string) =>
  `${((new Date(to).getTime() - new Date(from).getTime()) / 1000).toFixed(2)} s`

export function statusColor(status: string): string {
  switch (status) {
    case 'completed':
      return 'green'
    case 'failed':
      return 'red'
    case 'running':
      return 'blue'
    case 'cancelled':
      return 'gray'
    default:
      return 'yellow' // pending
  }
}

export default function Tasks() {
  const [tasks, setTasks] = useState<Task[]>([])
  const [page, setPage] = useState(1)
  const [totalPages, setTotalPages] = useState(1)
  const [total, setTotal] = useState(0)
  const [type, setType] = useState('send_email')
  const [priority, setPriority] = useState(3)
  const [count, setCount] = useState(1)
  const [mixed, setMixed] = useState(false) // random types/priorities per task
  const [submitting, setSubmitting] = useState(false)

  // Filters ("all" = no filter).
  const [clientFilter, setClientFilter] = useState('all')
  const [typeFilter, setTypeFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')

  // Detail drawer (a snapshot of the task clicked).
  const [selected, setSelected] = useState<Task | null>(null)
  const [drawerOpened, { open: openDrawer, close: closeDrawer }] = useDisclosure(false)

  const pageSize = 20

  // Load one page of tasks ACROSS ALL CLIENTS (ops view), applying any active filters,
  // refreshing every 2s so statuses update live as tasks run.
  const load = () => {
    const params = new URLSearchParams({ all: 'true', page: String(page), page_size: String(pageSize) })
    if (clientFilter !== 'all') params.set('client', clientFilter)
    if (typeFilter !== 'all') params.set('type', typeFilter)
    if (statusFilter !== 'all') params.set('status', statusFilter)
    api
      .get<TaskPage>(`/tasks?${params.toString()}`)
      .then((res) => {
        setTasks(res.data.data)
        setTotalPages(res.data.total_pages)
        setTotal(res.data.total)
      })
      .catch(() => {})
  }
  useEffect(() => {
    load()
    const id = setInterval(load, 2000)
    return () => clearInterval(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, clientFilter, typeFilter, statusFilter])

  // Changing a filter resets to page 1.
  const onFilter = (set: (v: string) => void) => (v: string | null) => {
    set(v || 'all')
    setPage(1)
  }

  const submit = async () => {
    setSubmitting(true)
    try {
      // Build `count` tasks. With "Random mix" on, each gets a random type + priority;
      // otherwise they all use the selected type + priority. Sent in one batch request.
      const tasks = Array.from({ length: count }, () =>
        mixed
          ? {
              type: TASK_TYPES[Math.floor(Math.random() * TASK_TYPES.length)],
              priority: Math.floor(Math.random() * 5) + 1,
            }
          : { type, priority },
      )
      await api.post('/tasks/batch', { tasks })
      notifications.show({
        message: `Submitted ${count} task${count > 1 ? 's' : ''} ${mixed ? '(random mix)' : `(${type})`}`,
        color: 'green',
      })
      load()
    } catch (err) {
      const status = (err as { response?: { status?: number } })?.response?.status
      notifications.show({
        message:
          status === 429
            ? 'Rate limit hit — too many tasks this minute. Wait a bit or submit fewer.'
            : 'Failed to submit tasks',
        color: 'red',
      })
    } finally {
      setSubmitting(false)
    }
  }

  const cancel = async (id: number) => {
    try {
      await api.post(`/tasks/${id}/cancel`)
      notifications.show({ message: `Cancelled task #${id}`, color: 'gray' })
      load()
    } catch {
      notifications.show({ message: `Could not cancel #${id} (already finished?)`, color: 'red' })
    }
  }

  return (
    <Stack gap="lg">
      <Title order={2}>Tasks</Title>

      {/* Submit form */}
      <Card withBorder radius="md" padding="md">
        <Group align="flex-end">
          <Select
            label="Type"
            data={TASK_TYPES}
            value={type}
            onChange={(v) => v && setType(v)}
            allowDeselect={false}
            disabled={mixed}
            w={200}
          />
          <NumberInput
            label="Priority"
            min={1}
            max={5}
            value={priority}
            onChange={(v) => setPriority(Number(v) || 3)}
            disabled={mixed}
            w={120}
          />
          <NumberInput
            label="Count"
            min={1}
            max={200}
            value={count}
            onChange={(v) => setCount(Number(v) || 1)}
            w={120}
          />
          <Switch
            label="Random mix"
            checked={mixed}
            onChange={(e) => setMixed(e.currentTarget.checked)}
            mb={8}
          />
          <Button onClick={submit} loading={submitting}>
            Submit {count > 1 ? `${count} tasks` : 'task'}
          </Button>
        </Group>
      </Card>

      {/* Filters */}
      <Card withBorder radius="md" padding="md">
        <Group>
          <Select
            label="Client"
            data={CLIENT_OPTIONS}
            value={clientFilter}
            onChange={onFilter(setClientFilter)}
            allowDeselect={false}
            w={160}
          />
          <Select
            label="Type"
            data={TYPE_OPTIONS}
            value={typeFilter}
            onChange={onFilter(setTypeFilter)}
            allowDeselect={false}
            w={180}
          />
          <Select
            label="Status"
            data={STATUS_OPTIONS}
            value={statusFilter}
            onChange={onFilter(setStatusFilter)}
            allowDeselect={false}
            w={160}
          />
        </Group>
      </Card>

      {/* Task table */}
      <Card withBorder radius="md" padding={0}>
        <Table.ScrollContainer minWidth={720}>
          <Table striped highlightOnHover verticalSpacing="sm" horizontalSpacing="md">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>ID</Table.Th>
                <Table.Th>Client</Table.Th>
                <Table.Th>Type</Table.Th>
                <Table.Th>Status</Table.Th>
                <Table.Th>Priority</Table.Th>
                <Table.Th>Retries</Table.Th>
                <Table.Th>Created</Table.Th>
                <Table.Th />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {tasks.map((t) => (
                <Table.Tr
                  key={t.id}
                  style={{ cursor: 'pointer' }}
                  onClick={() => {
                    setSelected(t)
                    openDrawer()
                  }}
                >
                  <Table.Td>{t.id}</Table.Td>
                  <Table.Td>
                    <Badge color={clientColor(t.client_id)} variant="light">
                      {t.client_id}
                    </Badge>
                  </Table.Td>
                  <Table.Td>{t.type}</Table.Td>
                  <Table.Td>
                    <Badge color={statusColor(t.status)} variant="light">
                      {t.status}
                    </Badge>
                  </Table.Td>
                  <Table.Td>{t.priority}</Table.Td>
                  <Table.Td>
                    {t.retry_count}/{t.max_retries}
                  </Table.Td>
                  <Table.Td>
                    <Text size="sm" c="dimmed">
                      {dayjs(t.created_at).fromNow()}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    {(t.status === 'pending' || t.status === 'running') && (
                      <Tooltip label="Cancel">
                        <ActionIcon
                        variant="subtle"
                        color="red"
                        onClick={(e) => {
                          e.stopPropagation() // don't open the drawer when cancelling
                          cancel(t.id)
                        }}
                      >
                          <IconX size={16} />
                        </ActionIcon>
                      </Tooltip>
                    )}
                  </Table.Td>
                </Table.Tr>
              ))}
              {tasks.length === 0 && (
                <Table.Tr>
                  <Table.Td colSpan={8}>
                    <Text c="dimmed" ta="center" py="md">
                      No tasks yet — submit one above, or hit "Demo data".
                    </Text>
                  </Table.Td>
                </Table.Tr>
              )}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      </Card>

      <Group justify="space-between">
        <Text size="sm" c="dimmed">
          {total} task{total === 1 ? '' : 's'} total
        </Text>
        {totalPages > 1 && <Pagination value={page} onChange={setPage} total={totalPages} />}
      </Group>

      {/* Per-task detail / lifecycle drawer */}
      <Drawer
        opened={drawerOpened}
        onClose={closeDrawer}
        position="right"
        size="md"
        title={selected ? `Task #${selected.id}` : ''}
      >
        {selected && (
          <Stack gap="sm">
            <Group>
              <Badge color={clientColor(selected.client_id)} variant="light">
                {selected.client_id}
              </Badge>
              <Badge color={statusColor(selected.status)} variant="light">
                {selected.status}
              </Badge>
            </Group>

            <DetailRow label="Type" value={selected.type} />
            <DetailRow label="Priority" value={selected.priority} />
            <DetailRow label="Retries" value={`${selected.retry_count} / ${selected.max_retries}`} />
            <DetailRow label="Processed by" value={selected.processed_by ?? '—'} />

            <Divider label="Timeline" labelPosition="left" />
            <DetailRow label="Created" value={fmtTime(selected.created_at)} />
            <DetailRow label="Started" value={selected.started_at ? fmtTime(selected.started_at) : '—'} />
            <DetailRow label="Completed" value={selected.completed_at ? fmtTime(selected.completed_at) : '—'} />

            <Divider label="Timings" labelPosition="left" />
            <DetailRow
              label="Queue wait"
              value={selected.started_at ? secsBetween(selected.created_at, selected.started_at) : '—'}
            />
            <DetailRow
              label="Execution"
              value={
                selected.started_at && selected.completed_at
                  ? secsBetween(selected.started_at, selected.completed_at)
                  : '—'
              }
            />

            {selected.error_message && (
              <>
                <Divider label="Error" labelPosition="left" color="red" />
                <Text size="sm" c="red">
                  {selected.error_message}
                </Text>
              </>
            )}
          </Stack>
        )}
      </Drawer>
    </Stack>
  )
}
