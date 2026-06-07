import { useEffect, useState } from 'react'
import {
  Card,
  Center,
  Group,
  Loader,
  SegmentedControl,
  SimpleGrid,
  Stack,
  Text,
  Title,
} from '@mantine/core'
import { AreaChart, BarChart, DonutChart, LineChart } from '@mantine/charts'

import { api, getClientId } from '../api'
import { StatCard } from '../StatCard'

// All numbers here come from the backend computed over the FULL set of rows (whole
// table, or one client) — never a recent sample.
interface Summary {
  total_tasks: number
  status_counts: Record<string, number>
  priority_counts: Record<string, number>
  failure_rate: number
  avg_execution_ms: number
  avg_queue_wait_ms: number
  completed_last_hour: number
}

interface ThroughputPoint {
  time: string
  completed: number
  avg_execution_ms: number
  avg_queue_wait_ms: number
}


const STATUS_COLORS: Record<string, string> = {
  completed: 'green.6',
  failed: 'red.6',
  pending: 'yellow.6',
  running: 'blue.6',
  cancelled: 'gray.5',
}

export default function Analytics() {
  const clientId = getClientId()
  const [scope, setScope] = useState<'all' | 'client'>('all')
  const [summary, setSummary] = useState<Summary | null>(null)
  const [throughput, setThroughput] = useState<ThroughputPoint[]>([])

  useEffect(() => {
    const q = scope === 'all' ? '?all=true' : ''
    const load = () => {
      api.get<Summary>(`/analytics${q}`).then((r) => setSummary(r.data)).catch(() => {})
      api.get<ThroughputPoint[]>(`/analytics/throughput${q}`).then((r) => setThroughput(r.data)).catch(() => {})
    }
    load()
    const id = setInterval(load, 5000)
    return () => clearInterval(id)
  }, [scope, clientId])

  const scopeControl = (
    <SegmentedControl
      value={scope}
      onChange={(v) => setScope(v as 'all' | 'client')}
      data={[
        { label: 'All clients', value: 'all' },
        { label: clientId, value: 'client' },
      ]}
    />
  )

  if (!summary) {
    return (
      <Stack gap="lg">
        <Group justify="space-between">
          <Title order={2}>Analytics</Title>
          {scopeControl}
        </Group>
        <Center h={160}>
          <Loader />
        </Center>
      </Stack>
    )
  }

  const donutData = Object.entries(summary.status_counts).map(([name, value]) => ({
    name,
    value,
    color: STATUS_COLORS[name] ?? 'gray.5',
  }))

  const priorityData = [1, 2, 3, 4, 5].map((p) => ({
    priority: `P${p}`,
    count: summary.priority_counts[String(p)] ?? 0,
  }))

  // Latency over time, derived from the throughput buckets (seconds).
  const latencyData = throughput.map((p) => ({
    time: p.time,
    'queue wait': +(p.avg_queue_wait_ms / 1000).toFixed(2),
    execution: +(p.avg_execution_ms / 1000).toFixed(2),
  }))

  return (
    <Stack gap="lg">
      <Group justify="space-between">
        <Title order={2}>Analytics</Title>
        {scopeControl}
      </Group>

      <SimpleGrid cols={{ base: 2, md: 4 }} spacing="md">
        <StatCard label="Total tasks" value={summary.total_tasks} />
        <StatCard label="Failure rate" value={`${(summary.failure_rate * 100).toFixed(1)}%`} />
        <StatCard label="Avg execution" value={`${(summary.avg_execution_ms / 1000).toFixed(2)} s`} />
        <StatCard label="Avg queue wait" value={`${(summary.avg_queue_wait_ms / 1000).toFixed(2)} s`} />
      </SimpleGrid>

      <Card withBorder radius="md" padding="lg">
        <Text fw={600} mb="md">
          Throughput (completed per 10s, last 5 min)
        </Text>
        <AreaChart
          h={220}
          data={throughput}
          dataKey="time"
          series={[{ name: 'completed', color: 'teal.6' }]}
          curveType="monotone"
          withDots={false}
        />
      </Card>

      <Card withBorder radius="md" padding="lg">
        <Text fw={600} mb="md">
          Latency over time (queue wait vs execution)
        </Text>
        <LineChart
          h={220}
          data={latencyData}
          dataKey="time"
          unit="s"
          series={[
            { name: 'queue wait', color: 'orange.6' },
            { name: 'execution', color: 'blue.6' },
          ]}
          curveType="monotone"
          withDots={false}
        />
      </Card>

      <SimpleGrid cols={{ base: 1, md: 2 }} spacing="md">
        <Card withBorder radius="md" padding="lg">
          <Text fw={600} mb="md">
            Status distribution
          </Text>
          <Group justify="center">
            <DonutChart data={donutData} withTooltip size={200} thickness={32} chartLabel={`${summary.total_tasks} total`} />
          </Group>
        </Card>

        <Card withBorder radius="md" padding="lg">
          <Text fw={600} mb="md">
            Tasks by priority
          </Text>
          <BarChart h={220} data={priorityData} dataKey="priority" series={[{ name: 'count', color: 'grape.6' }]} />
        </Card>
      </SimpleGrid>
    </Stack>
  )
}
