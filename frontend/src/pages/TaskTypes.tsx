import { useEffect, useState } from 'react'
import { Badge, Card, Group, SegmentedControl, Stack, Table, Text, Title } from '@mantine/core'
import { BarChart } from '@mantine/charts'

import { api, getClientId } from '../api'

// Per-task-type breakdown (matches TypeStat in Go).
interface TypeStat {
  type: string
  total: number
  completed: number
  failed: number
  pending: number
  running: number
  cancelled: number
  failure_rate: number
  avg_execution_ms: number
  avg_queue_wait_ms: number
}

const pct = (n: number, total: number) => (total > 0 ? `${((n / total) * 100).toFixed(1)}%` : '0%')

export default function TaskTypes() {
  const clientId = getClientId()
  const [scope, setScope] = useState<'all' | 'client'>('all')
  const [stats, setStats] = useState<TypeStat[]>([])

  useEffect(() => {
    const q = scope === 'all' ? '?all=true' : ''
    const load = () => api.get<TypeStat[]>(`/analytics/types${q}`).then((r) => setStats(r.data)).catch(() => {})
    load()
    const id = setInterval(load, 5000)
    return () => clearInterval(id)
  }, [scope, clientId])

  // Stacked-bar data: outcome counts per type.
  const chartData = stats.map((t) => ({
    type: t.type,
    completed: t.completed,
    failed: t.failed,
    pending: t.pending + t.running,
    cancelled: t.cancelled,
  }))

  return (
    <Stack gap="lg">
      <Group justify="space-between">
        <Title order={2}>Task Types</Title>
        <SegmentedControl
          value={scope}
          onChange={(v) => setScope(v as 'all' | 'client')}
          data={[
            { label: 'All clients', value: 'all' },
            { label: clientId, value: 'client' },
          ]}
        />
      </Group>

      {/* Outcomes per type (stacked) */}
      <Card withBorder radius="md" padding="lg">
        <Text fw={600} mb="md">
          Outcomes by type
        </Text>
        <BarChart
          h={280}
          data={chartData}
          dataKey="type"
          type="stacked"
          series={[
            { name: 'completed', color: 'green.6' },
            { name: 'failed', color: 'red.6' },
            { name: 'pending', color: 'yellow.6' },
            { name: 'cancelled', color: 'gray.5' },
          ]}
        />
      </Card>

      {/* Per-type detail table */}
      <Card withBorder radius="md" padding="lg">
        <Text fw={600} mb="md">
          Per-type detail
        </Text>
        <Table.ScrollContainer minWidth={680}>
          <Table verticalSpacing="sm" highlightOnHover>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Type</Table.Th>
                <Table.Th>Total</Table.Th>
                <Table.Th>Completed</Table.Th>
                <Table.Th>Failed</Table.Th>
                <Table.Th>Failure rate</Table.Th>
                <Table.Th>Avg execution</Table.Th>
                <Table.Th>Avg queue wait</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {stats.map((t) => (
                <Table.Tr key={t.type}>
                  <Table.Td>{t.type}</Table.Td>
                  <Table.Td>{t.total}</Table.Td>
                  <Table.Td>
                    {t.completed}{' '}
                    <Text span c="dimmed" size="xs">
                      ({pct(t.completed, t.total)})
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    {t.failed}{' '}
                    <Text span c="dimmed" size="xs">
                      ({pct(t.failed, t.total)})
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Badge variant="light" color={t.failure_rate > 0.25 ? 'red' : 'green'}>
                      {(t.failure_rate * 100).toFixed(1)}%
                    </Badge>
                  </Table.Td>
                  <Table.Td>{(t.avg_execution_ms / 1000).toFixed(2)} s</Table.Td>
                  <Table.Td>{(t.avg_queue_wait_ms / 1000).toFixed(2)} s</Table.Td>
                </Table.Tr>
              ))}
              {stats.length === 0 && (
                <Table.Tr>
                  <Table.Td colSpan={7}>
                    <Text c="dimmed" ta="center" py="md">
                      No tasks yet.
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
