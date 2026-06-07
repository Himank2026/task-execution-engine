import { useEffect, useState } from 'react'
import { Card, Center, Group, Loader, SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { AreaChart, BarChart, DonutChart, LineChart } from '@mantine/charts'

import { api, type Task, type TaskPage } from '../api'
import { StatCard } from '../StatCard'

interface Summary {
  total_tasks: number
  status_counts: Record<string, number>
  failure_rate: number
  avg_execution_ms: number
  avg_queue_wait_ms: number
  completed_last_hour: number
}

// One 10-second bucket on the throughput chart (matches ThroughputPoint in Go).
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

// Milliseconds between two ISO timestamps, in seconds (2 decimals).
function secondsBetween(from: string, to: string): number {
  return +((new Date(to).getTime() - new Date(from).getTime()) / 1000).toFixed(2)
}

export default function Analytics() {
  const [summary, setSummary] = useState<Summary | null>(null)
  const [tasks, setTasks] = useState<Task[]>([])
  const [throughput, setThroughput] = useState<ThroughputPoint[]>([])

  useEffect(() => {
    const load = () => {
      api.get<Summary>('/analytics').then((r) => setSummary(r.data)).catch(() => {})
      // pull a larger sample (up to 100) so the per-task charts have data
      api.get<TaskPage>('/tasks?page_size=100').then((r) => setTasks(r.data.data)).catch(() => {})
      api.get<ThroughputPoint[]>('/analytics/throughput').then((r) => setThroughput(r.data)).catch(() => {})
    }
    load()
    const id = setInterval(load, 5000)
    return () => clearInterval(id)
  }, [])

  if (!summary) {
    return (
      <Center h={200}>
        <Loader />
      </Center>
    )
  }

  // Status distribution (from the aggregate counts).
  const donutData = Object.entries(summary.status_counts).map(([name, value]) => ({
    name,
    value,
    color: STATUS_COLORS[name] ?? 'gray.5',
  }))

  // Per-task latency: queue wait + execution time, for the most recent completed tasks.
  const latencyData = tasks
    .filter((t) => t.status === 'completed' && t.started_at && t.completed_at)
    .slice(0, 30) // newest 30
    .reverse() // oldest-first so the line reads left → right
    .map((t) => ({
      task: `#${t.id}`,
      'queue wait': secondsBetween(t.created_at, t.started_at!),
      execution: secondsBetween(t.started_at!, t.completed_at!),
    }))

  // Submissions by priority (1–5).
  const priorityData = [1, 2, 3, 4, 5].map((p) => ({
    priority: `P${p}`,
    count: tasks.filter((t) => t.priority === p).length,
  }))

  return (
    <Stack gap="lg">
      <Title order={2}>Analytics</Title>

      {/* Headline numbers */}
      <SimpleGrid cols={{ base: 2, md: 4 }} spacing="md">
        <StatCard label="Total tasks" value={summary.total_tasks} />
        <StatCard label="Failure rate" value={`${(summary.failure_rate * 100).toFixed(1)}%`} />
        <StatCard label="Avg execution" value={`${(summary.avg_execution_ms / 1000).toFixed(2)} s`} />
        <StatCard label="Avg queue wait" value={`${(summary.avg_queue_wait_ms / 1000).toFixed(2)} s`} />
      </SimpleGrid>

      {/* Throughput over time — completions per 10-second bucket, last 5 minutes */}
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

      {/* Latency per task — queue wait vs execution time */}
      <Card withBorder radius="md" padding="lg">
        <Text fw={600} mb="md">
          Latency per task (queue wait vs execution)
        </Text>
        {latencyData.length === 0 ? (
          <Text c="dimmed" size="sm">
            No completed tasks yet — submit some on the Tasks page.
          </Text>
        ) : (
          <LineChart
            h={260}
            data={latencyData}
            dataKey="task"
            unit="s"
            series={[
              { name: 'queue wait', color: 'orange.6' },
              { name: 'execution', color: 'blue.6' },
            ]}
            curveType="monotone"
            withDots={false}
          />
        )}
      </Card>

      {/* Status distribution + priority spread side by side */}
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
