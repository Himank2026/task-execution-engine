import { useEffect, useState } from 'react'
import {
  Badge,
  Card,
  Group,
  Loader,
  ScrollArea,
  SimpleGrid,
  Stack,
  Text,
  Title,
} from '@mantine/core'
import dayjs from 'dayjs'

import { api, clientColor } from '../api'
import { useSSE } from '../useSSE'
import { StatCard } from '../StatCard'

// Matches the JSON from GET /api/analytics (AnalyticsService.Summary in Go).
interface Summary {
  total_tasks: number
  status_counts: Record<string, number>
  failure_rate: number
  avg_execution_ms: number
  avg_queue_wait_ms: number
  completed_last_hour: number
}

// Matches GET /api/workers (worker.WorkerStatus in Go).
interface WorkerStatus {
  id: number
  busy: boolean
  task_id?: number
  task_type?: string
  client_id?: string
  busy_ms?: number
}

// One backend instance and its workers (the panel groups by this).
interface InstanceWorkers {
  instance: string
  workers: WorkerStatus[]
}

// Colour the event badge by what happened.
function eventColor(type: string): string {
  if (type === 'task.completed') return 'green'
  if (type === 'task.failed') return 'red'
  return 'blue'
}

export default function Dashboard() {
  const [summary, setSummary] = useState<Summary | null>(null)
  const [instances, setInstances] = useState<InstanceWorkers[]>([])
  const { events, connected } = useSSE() // live task feed

  // Poll worker state every second. The backend aggregates ALL instances from Redis, so
  // this shows every backend's workers — not just the one we queried.
  useEffect(() => {
    const load = () => {
      api
        .get<{ instances: InstanceWorkers[] }>('/workers')
        .then((r) => setInstances(r.data.instances))
        .catch(() => {})
    }
    load()
    const id = setInterval(load, 1000)
    return () => clearInterval(id)
  }, [])

  const totalWorkers = instances.reduce((n, i) => n + i.workers.length, 0)

  // Load the analytics summary on mount, then refresh every 5s so the cards stay current.
  useEffect(() => {
    let active = true
    const load = () => {
      api
        .get<Summary>('/analytics?all=true')
        .then((res) => {
          if (active) setSummary(res.data)
        })
        .catch(() => {
          /* backend may be down; keep last values */
        })
    }
    load()
    const id = setInterval(load, 5000)
    return () => {
      active = false
      clearInterval(id)
    }
  }, [])

  return (
    <Stack gap="lg">
      <Group justify="space-between">
        <Title order={2}>Dashboard</Title>
        <Badge color={connected ? 'green' : 'gray'} variant="light">
          {connected ? 'Live' : 'Connecting…'}
        </Badge>
      </Group>

      {/* Summary stat cards. The cols object IS the responsiveness: 1 column on phones,
          2 on small screens, 4 on medium+. */}
      {!summary ? (
        <Loader />
      ) : (
        <SimpleGrid cols={{ base: 1, xs: 2, md: 4 }} spacing="md">
          <StatCard label="Total tasks" value={summary.total_tasks} />
          <StatCard label="Completed" value={summary.status_counts.completed ?? 0} />
          <StatCard label="Failed" value={summary.status_counts.failed ?? 0} />
          <StatCard label="Pending" value={summary.status_counts.pending ?? 0} />
          <StatCard label="Failure rate" value={`${(summary.failure_rate * 100).toFixed(1)}%`} />
          <StatCard label="Avg execution" value={`${(summary.avg_execution_ms / 1000).toFixed(2)} s`} />
          <StatCard label="Avg queue wait" value={`${(summary.avg_queue_wait_ms / 1000).toFixed(2)} s`} />
          <StatCard label="Completed / hour" value={summary.completed_last_hour} />
        </SimpleGrid>
      )}

      {/* Worker pool across ALL backend instances (aggregated from Redis). */}
      <Card withBorder radius="md" padding="md">
        <Group justify="space-between" mb="sm">
          <Text fw={600}>Workers</Text>
          <Text size="sm" c="dimmed">
            {instances.length} instance{instances.length === 1 ? '' : 's'} · {totalWorkers} workers
          </Text>
        </Group>

        <Stack gap="md">
          {instances.map((inst) => (
            <div key={inst.instance}>
              <Group gap="xs" mb="xs">
                <Badge variant="filled" color="indigo">
                  {inst.instance}
                </Badge>
                <Text size="xs" c="dimmed">
                  {inst.workers.filter((w) => w.busy).length}/{inst.workers.length} busy
                </Text>
              </Group>
              <SimpleGrid cols={{ base: 2, sm: 4 }} spacing="sm">
                {inst.workers.map((w) => (
                  <Card
                    key={w.id}
                    withBorder
                    radius="md"
                    padding="sm"
                    bg={w.busy ? 'var(--mantine-primary-color-light)' : undefined}
                  >
                    <Group justify="space-between">
                      <Text fw={600} size="sm">
                        Worker {w.id}
                      </Text>
                      <Badge size="sm" variant="light" color={w.busy ? 'blue' : 'gray'}>
                        {w.busy ? 'Busy' : 'Idle'}
                      </Badge>
                    </Group>
                    {w.busy ? (
                      <Stack gap={4} mt={8}>
                        <Group gap={6}>
                          <Text size="sm">task #{w.task_id}</Text>
                          <Badge size="xs" variant="light" color={clientColor(w.client_id ?? '')}>
                            {w.client_id}
                          </Badge>
                        </Group>
                        <Text size="xs" c="dimmed">
                          {w.task_type} · running {((w.busy_ms ?? 0) / 1000).toFixed(1)}s
                        </Text>
                      </Stack>
                    ) : (
                      <Text size="sm" c="dimmed" mt={8}>
                        idle
                      </Text>
                    )}
                  </Card>
                ))}
              </SimpleGrid>
            </div>
          ))}
          {instances.length === 0 && (
            <Text c="dimmed" size="sm">
              No instances reporting yet…
            </Text>
          )}
        </Stack>
      </Card>

      {/* Live activity feed, fed by SSE. */}
      <Card withBorder radius="md" padding="md">
        <Text fw={600} mb="sm">
          Live activity
        </Text>
        {events.length === 0 ? (
          <Text c="dimmed" size="sm">
            Waiting for task events… submit some tasks to see them stream in here.
          </Text>
        ) : (
          <ScrollArea h={320}>
            <Stack gap="xs">
              {events.map((e, i) => (
                <Group key={i} justify="space-between" wrap="nowrap">
                  <Group gap="sm" wrap="nowrap">
                    <Badge color={eventColor(e.type)} variant="light" w={110}>
                      {e.type.replace('task.', '')}
                    </Badge>
                    <Text size="sm">task #{e.task_id}</Text>
                  </Group>
                  <Text size="xs" c="dimmed">
                    {dayjs.unix(e.time).format('HH:mm:ss')}
                  </Text>
                </Group>
              ))}
            </Stack>
          </ScrollArea>
        )}
      </Card>
    </Stack>
  )
}
