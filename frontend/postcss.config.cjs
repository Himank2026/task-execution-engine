// Mantine's styling needs PostCSS. This config enables Mantine's CSS features and
// defines the responsive breakpoints we can reference in styles (e.g. for media
// queries). You set this up once and basically never touch it again.
module.exports = {
  plugins: {
    'postcss-preset-mantine': {},
    'postcss-simple-vars': {
      variables: {
        'mantine-breakpoint-xs': '36em',
        'mantine-breakpoint-sm': '48em',
        'mantine-breakpoint-md': '62em',
        'mantine-breakpoint-lg': '75em',
        'mantine-breakpoint-xl': '88em',
      },
    },
  },
}
