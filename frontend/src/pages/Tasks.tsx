import { useEffect, useState } from 'react'
import {
  ActionIcon,
  Badge,
  Button,
  Card,
  Group,
  NumberInput,
  Select,
  Stack,
  Table,
  Text,
  Title,
  Tooltip,
} from '@mantine/core'
import { notifications } from '@mantine/notifications'
import { IconX } from '@tabler/icons-react'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'

import { api, type Task, type TaskPage } from '../api'

dayjs.extend(relativeTime) // enables "3 seconds ago" formatting

const TASK_TYPES = ['send_email', 'send_sms', 'generate_report', 'resize_image']

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
  const [type, setType] = useState('send_email')
  const [priority, setPriority] = useState(3)
  const [submitting, setSubmitting] = useState(false)

  // Load the latest tasks, and keep refreshing so statuses update as they run.
  const load = () => {
    api
      .get<TaskPage>('/tasks')
      .then((res) => setTasks(res.data.data))
      .catch(() => {})
  }
  useEffect(() => {
    load()
    const id = setInterval(load, 2000)
    return () => clearInterval(id)
  }, [])

  const submit = async () => {
    setSubmitting(true)
    try {
      await api.post('/tasks', { type, priority })
      notifications.show({ message: `Submitted a "${type}" task`, color: 'green' })
      load()
    } catch {
      notifications.show({ message: 'Failed to submit task', color: 'red' })
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
            w={200}
          />
          <NumberInput
            label="Priority"
            min={1}
            max={5}
            value={priority}
            onChange={(v) => setPriority(Number(v) || 3)}
            w={120}
          />
          <Button onClick={submit} loading={submitting}>
            Submit task
          </Button>
        </Group>
      </Card>

      {/* Task table */}
      <Card withBorder radius="md" padding={0}>
        <Table.ScrollContainer minWidth={720}>
          <Table striped highlightOnHover verticalSpacing="sm" horizontalSpacing="md">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>ID</Table.Th>
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
                <Table.Tr key={t.id}>
                  <Table.Td>{t.id}</Table.Td>
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
                        <ActionIcon variant="subtle" color="red" onClick={() => cancel(t.id)}>
                          <IconX size={16} />
                        </ActionIcon>
                      </Tooltip>
                    )}
                  </Table.Td>
                </Table.Tr>
              ))}
              {tasks.length === 0 && (
                <Table.Tr>
                  <Table.Td colSpan={7}>
                    <Text c="dimmed" ta="center" py="md">
                      No tasks yet — submit one above.
                    </Text>
                  </Table.Td>
                </Table.Tr>
              )}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      </Card>
    </Stack>
  )
}
