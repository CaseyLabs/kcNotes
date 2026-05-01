/*
  Tailwind configuration file.

  JavaScript concept: JSDoc type import
  - The next line is not runtime code; it helps editors provide autocomplete
    and type hints.
  - `import('tailwindcss').Config` describes the shape of this object.
*/
/** @type {import('tailwindcss').Config} */
module.exports = {
  /*
    `content` tells Tailwind where to scan for class names.

    JavaScript concept: arrays and strings
    - `[...]` is an array.
    - Each string is a glob pattern.

    Project concept:
    - Our UI templates are Go templates under `web/templates`.
    - Tailwind reads those files, finds classes, and emits only CSS we use.
    - This keeps CSS output small.
  */
  content: ["./web/templates/**/*.tmpl"],

  /*
    `theme` controls design tokens (colors, spacing, fonts, etc).
    `extend` merges custom values into Tailwind defaults.

    Empty `extend` means:
    - we currently use Tailwind defaults,
    - but this is the standard place to add custom design tokens later.
  */
  theme: {
    extend: {}
  },

  /*
    Tailwind plugins array.
    - Add official/community plugins here.
    - Empty array means no extra Tailwind plugins are enabled right now.
  */
  plugins: []
};
