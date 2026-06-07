import { AppShell, Burger, Button, Group, NavLink, Select, Title } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { notifications } from '@mantine/notifications'
import { NavLink as RouterNavLink, Route, Routes, useLocation } from 'react-router-dom'
import { IconChartBar, IconLayoutDashboard, IconListCheck } from '@tabler/icons-react'

import { api, API_KEYS, getApiKey, setApiKey } from './api'
import Dashboard from './pages/Dashboard'
import Tasks from './pages/Tasks'
import Analytics from './pages/Analytics'

// The three screens in the sidebar.
const navItems = [
  { to: '/', label: 'Dashboard', icon: IconLayoutDashboard },
  { to: '/tasks', label: 'Tasks', icon: IconListCheck },
  { to: '/analytics', label: 'Analytics', icon: IconChartBar },
]

export default function App() {
  // Controls the mobile sidebar (open/closed). On desktop the sidebar is always shown.
  const [opened, { toggle, close }] = useDisclosure()
  const location = useLocation()

  // Wipe this client's tasks and seed 50 fresh random ones — a clean slate for demos
  // without the table accumulating test rows. The pages poll, so they pick up the new
  // data within a couple seconds.
  const generateDemo = async () => {
    try {
      await api.post('/demo/reset')
      notifications.show({ message: 'Generated 50 fresh demo tasks', color: 'teal' })
    } catch {
      notifications.show({ message: 'Failed to generate demo data', color: 'red' })
    }
  }

  return (
    <AppShell
      header={{ height: 60 }}
      // breakpoint: 'sm' => below ~48em the sidebar collapses and the burger appears.
      // That's our responsiveness, handled by Mantine — no manual media queries needed.
      navbar={{ width: 240, breakpoint: 'sm', collapsed: { mobile: !opened } }}
      padding="md"
    >
      {/* Top bar: burger (mobile only) + app name on the left, client switcher on the right */}
      <AppShell.Header>
        <Group h="100%" px="md" justify="space-between">
          <Group>
            <Burger opened={opened} onClick={toggle} hiddenFrom="sm" size="sm" />
            <Title order={3}>Task Engine</Title>
          </Group>
          <Group gap="xs" wrap="nowrap">
            <Button variant="light" size="sm" onClick={generateDemo}>
              Demo data
            </Button>
            <Select
              aria-label="Acting as client"
              data={API_KEYS}
              defaultValue={getApiKey()}
              allowDeselect={false}
              w={140}
              onChange={(value) => {
                if (value) {
                  setApiKey(value)
                  window.location.reload() // simplest way to refetch everything as the new client
                }
              }}
            />
          </Group>
        </Group>
      </AppShell.Header>

      {/* Left sidebar navigation */}
      <AppShell.Navbar p="md">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            component={RouterNavLink}
            to={item.to}
            label={item.label}
            leftSection={<item.icon size={18} />}
            active={location.pathname === item.to}
            onClick={close} // close the mobile sidebar after navigating
          />
        ))}
      </AppShell.Navbar>

      {/* The page content — whichever route matches shows here */}
      <AppShell.Main>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/tasks" element={<Tasks />} />
          <Route path="/analytics" element={<Analytics />} />
        </Routes>
      </AppShell.Main>
    </AppShell>
  )
}
