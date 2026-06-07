import { Card, Text } from '@mantine/core'

// A small reusable stat tile, shared by the Dashboard and Analytics pages.
export function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <Card withBorder padding="md" radius="md">
      <Text size="xs" c="dimmed" tt="uppercase" fw={600}>
        {label}
      </Text>
      <Text size="xl" fw={700} mt={4}>
        {value}
      </Text>
    </Card>
  )
}
