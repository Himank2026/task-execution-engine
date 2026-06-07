import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { createTheme, MantineProvider } from '@mantine/core'
import { Notifications } from '@mantine/notifications'

// Mantine and its add-ons ship their own CSS — import them once here, at the top.
import '@mantine/core/styles.css'
import '@mantine/charts/styles.css'
import '@mantine/notifications/styles.css'
import './index.css'

import App from './App'

// App-wide theme: indigo as the primary accent (buttons, active nav, charts, badges).
const theme = createTheme({ primaryColor: 'indigo' })

// This file just wires up the "providers" that everything else needs:
//   MantineProvider  -> makes Mantine components + theme available
//   Notifications    -> enables the toast popups
//   BrowserRouter    -> enables page navigation (the URL bar)
// You rarely touch this file again.
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <MantineProvider theme={theme} defaultColorScheme="dark">
      <Notifications />
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </MantineProvider>
  </StrictMode>,
)
